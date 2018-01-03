package main

import (
	"os"
	"testing"

	"github.com/go-test/deep"
)

var (
	club        = os.Setenv("CLUB_NAME", "dev")
	sqsQueueURL = os.Setenv("SQS_QUEUE_URL", "https://foo/bar")
	statsdHost  = os.Setenv("STATSD_HOST", "127.0.0.1:8125")
)

func TestNumLoop(t *testing.T) {
	var data = []struct {
		n        int
		expected int
	}{
		{1, 3},
		{2, 6},
		{3, 9},
	}

	for _, tt := range data {
		// numLoop accepts a int and return 3 goroutines * this int.
		// Minimum goroutine is 3 because we have 3 different pipelines
		// one for read SQS message, parse messasge ( product metric)
		// and delete message from SQS.
		actual := numLoop(tt.n)
		if actual != tt.expected {
			t.Errorf("numLoop(%d): expected %d, actual %d", tt.n, tt.expected, actual)
		}
	}
}

func TestMetricTag(t *testing.T) {
	var data = []struct {
		key      string
		val      string
		expected string
	}{
		{"club_name", "foo", "club_name:foo"},
		{"x_edge_location", "MEL50", "x_edge_location:MEL50"},
		{"foo", "bar", "foo:bar"},
	}
	for _, tt := range data {
		actual := metricTag(tt.key, tt.val)
		if actual != tt.expected {
			t.Errorf("metricTag(%s, %s): expected %v, actual %v",
				tt.key,
				tt.val,
				tt.expected,
				actual)
		}
	}
}

func TestGetString(t *testing.T) {
	var data = []struct {
		msg      string
		val      string
		expected string
	}{
		{`{"x-edge-location":"SYD1","c-ip":"1.8.1.160","cs-uri-stem":"/8d41/2015/01/22/00.bin","sc-status":"200","cs-protocol":"https","x-edge-response-result-type":"Miss","type":"cloudfront.access"}`, "c-ip", "1.8.1.160"},
		{`{"x-edge-location":"MEL50",cs-uri-stem":"/8d41/2015/01/22/00.bin","sc-status":"200","x-edge-result-type":"Miss","cs-protocol":"https","x-edge-response-result-type":"Miss","type":"cloudfront.access"}`, "x-edge-location", "MEL50"},
		{`{"x-edge-location":"SYD1","cs-uri-stem":"/8d41/2015/01/22/00.bin","sc-status":"200","x-edge-result-type":"Miss","cs-protocol":"https","x-edge-response-result-type":"Miss","type":"cloudfront.access"}`, "x-edge-response-result-type", "Miss"},
	}

	for _, tt := range data {
		actual := getString(tt.msg, tt.val)
		if actual != tt.expected {
			t.Errorf("getString(%s, %s): expected %v, actual %v",
				tt.msg,
				tt.val,
				tt.expected,
				actual)
		}
	}
}

func TestGetFloat(t *testing.T) {
	var data = []struct {
		msg      string
		val      string
		expected float64
	}{
		{`{"x-edge-location":"SYD1","c-ip":"1.8.1.160","time-taken":"1.14","type":"cloudfront.access"}`, "time-taken", 1.14},
		{`{"x-edge-location":"SYD1","c-ip":"1.8.1.160","time-taken":"0.923","type":"cloudfront.access"}`, "time-taken", 0.923},
	}

	for _, tt := range data {
		actual := getFloat(tt.msg, tt.val)
		if actual != tt.expected {
			t.Errorf("getFloat(%s, %s): expected %v, actual %v",
				tt.msg,
				tt.val,
				tt.expected,
				actual)
		}
	}
}

func compare(l []string, v ...string) []string {
	var list []string
	for _, i := range l {
		for _, j := range v {
			if i == j {
				list = append(list, j)
			}
		}
	}
	return list
}

func TestMetricTags(t *testing.T) {
	var data = []struct {
		val1     string
		val2     string
		expected []string
	}{
		{"foo:123", "bar:456", []string{"foo:123", "bar:456"}},
		{"edge:MEL50", "hit:Miss", []string{"edge:MEL50", "hit:Miss"}},
	}

	for _, tt := range data {
		actual := metricTags(tt.val1, tt.val2)
		if len(actual) == len(tt.expected) {
			if diff := deep.Equal(actual, tt.expected); diff != nil {
				t.Errorf("metricTags(%s, %s): expected %v, actual %v",
					tt.val1,
					tt.val2,
					tt.expected,
					actual)
			}
		}
	}
}
