package main

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	statsd "github.com/DataDog/datadog-go/statsd"
	"github.com/joeshaw/envdecode"
	"github.com/tidwall/gjson"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

// Version in reality, I would like this to match Git tag but I am not sure
// how I would go about this. Hence, for now we just remember to
// bump version to match Git tag version we plan to create.
const Version = "0.0.8"

var config struct {
	GoRoutine                int    `env:"GOROUTINE,default=1"`
	ChannelBufferSize        int64  `env:"CHANNEL_BUFFER_SIZE,default=10"`
	Club                     string `env:"CLUB_NAME,required"`
	SqsRegion                string `env:"SQS_REGION,default=us-west-2"`
	SqsQueueURL              string `env:"SQS_QUEUE_URL,required"`
	SqsWaitTimeSeconds       int64  `env:"SQS_WAIT_TIME_SECONDS,default=10"`
	SqsMaxNumberOfMessages   int64  `env:"SQS_MAX_NUMBER_OF_MESSAGES,default=10"`
	SqsVisibilityTimeout     int64  `env:"SQS_VISIBILITY_TIMEOUT,default=300"`
	SqsMessageAttributeNames string `env:"SQS_MESSAGE_ATTRIBUTE_NAME,default=cloudfront"`
	// StatsdHost format host:port. Eg. 127.0.0.1:8125
	// Only supports UDP since we rely on dogstatsd/datadog agent config.
	StatsdHost        string `env:"STATSD_HOST,required"`
	StatsdPrefix      string `env:"STATSD_PREFIX,default=cloudfront."`
	HeartbeatInterval int    `env:"HEARTBEAT_INTERVAL,default=5"`
	HeartbeatTimeout  int    `env:"HEARTBEAT_INTERVAL,default=10"`
}

type queue struct {
	URL     string
	Message *sqs.Message
}

const (
	minGoroutineCount = 3
	sourceApp         = "cloudfront_log_metric_parser"
)

func init() {
	if err := envdecode.Decode(&config); err != nil {
		log.Fatalf("%s\n", err.Error())
	}
}

func main() {
	// We log everything in stdout which is helpful for debugging.
	log.Printf("%s\n", "cloudfront metric generator started")

	// TODO: Fix below hardcoded version. Would sticking
	// below in .env work well maybe?
	log.Printf("version %s\n", Version)

	log.Printf("%s %d\n", "Goroutine set to", config.GoRoutine)

	var wg sync.WaitGroup
	region := config.SqsRegion
	conf := &aws.Config{
		Region: &region,
	}

	sess, err := session.NewSession(conf)
	if err != nil {
		panic(err)
	}
	svc := sqs.New(sess)

	m, err := statsd.New(config.StatsdHost)
	if err != nil {
		log.Fatal(err)
	}
	// prefix every metric with the app name
	m.Namespace = config.StatsdPrefix

	messageStreamInput := make(chan *sqs.Message, config.ChannelBufferSize)
	deleteMessageStream := make(chan *string, config.ChannelBufferSize)
	aliveParser := make(chan string)
	aliveDelete := make(chan string)
	// We do +1 below because i starts from non zero.
	for i := minGoroutineCount; i <= numLoop(config.GoRoutine); i += minGoroutineCount {
		wg.Add(i)
		go receiveMessage(m, svc, messageStreamInput, &wg)
		go parseMessage(m, messageStreamInput, deleteMessageStream, &wg, aliveParser)
		go deleteMessage(m, svc, deleteMessageStream, &wg, aliveDelete)
		go heartbeatParse(m, aliveParser, &wg)
		go heartbeatDelete(m, aliveDelete, &wg)
	}
	wg.Wait()
	log.Printf("%s\n", "cloudfront metric generator stopped")
}

func numLoop(v int) int {
	return (minGoroutineCount * v)
}

func receiveMessage(d *statsd.Client, svc *sqs.SQS, messageStreamInput chan *sqs.Message, wg *sync.WaitGroup) {
	defer wg.Done()

	params := &sqs.ReceiveMessageInput{
		QueueUrl:            &config.SqsQueueURL,
		WaitTimeSeconds:     &config.SqsWaitTimeSeconds,
		MaxNumberOfMessages: &config.SqsMaxNumberOfMessages,
		VisibilityTimeout:   &config.SqsVisibilityTimeout,
		MessageAttributeNames: []*string{
			aws.String(config.SqsMessageAttributeNames),
		},
	}

	for {
		resp, err := svc.ReceiveMessage(params)
		if err != nil {
			log.Fatalf("%v\n%v", params, err)
		}

		for _, i := range resp.Messages {
			log.Printf("%s", "receive message from SQS")
			sendEvent(d, statsd.Event{
				Title: "recieve SQS message",
				Text:  fmt.Sprintf("%s %s", "received message from queue", config.SqsQueueURL),
			})
			messageStreamInput <- i
		}
	}
}

func appendTags(values ...string) []string {
	var tags []string
	for _, value := range values {
		tags = append(tags, value)
	}
	return tags
}

func createTag(k, v string) string {
	return k + ":" + v
}

func getString(msg, v string) string {
	return gjson.Get(msg, v).String()
}

func getFloat(msg, v string) float64 {
	return gjson.GetBytes([]byte(msg), v).Float()
}

func parseMessage(d *statsd.Client,
	messageStreamInput <-chan *sqs.Message,
	deleteMessageStream chan<- *string,
	wg *sync.WaitGroup,
	aliveParser chan<- string) {
	wg.Done()
	for {
		select {
		case msg := <-messageStreamInput:

			var err error
			cIP := getString(string(*msg.Body), "c-ip")
			timeTaken := getFloat(string(*msg.Body), "time-taken")
			csURIStem := getString(string(*msg.Body), "cs-uri-stem")
			xEdgeLocation := getString(string(*msg.Body), "x-edge-location")
			xEdgeResultType := getString(string(*msg.Body), "x-edge-result-type")
			date := getString(string(*msg.Body), "date")
			time := getString(string(*msg.Body), "time")
			csMethod := getString(string(*msg.Body), "cs-method")
			scStatus := getString(string(*msg.Body), "sc-status")
			csURIQuery := getString(string(*msg.Body), "cs-uri-query")
			xEdgeRequestID := getString(string(*msg.Body), "x-edge-request-id")
			xHostHeader := getString(string(*msg.Body), "x-host-header")
			csProtocol := getString(string(*msg.Body), "cs-protocol")
			xff := getString(string(*msg.Body), "x-forwarded-for")
			sslProto := getString(string(*msg.Body), "ssl-protocol")
			sslCipher := getString(string(*msg.Body), "ssl_cipher")
			xEdgeResponseResultType := getString(string(*msg.Body), "x-edge-response-result-type")
			csProtocolVersion := getString(string(*msg.Body), "cs-protocol-version")
			fleStatus := getString(string(*msg.Body), "fle-status")
			fleEncryptedFields := getString(string(*msg.Body), "fle-encrypted-fields")
			csHost := getString(string(*msg.Body), "cs(Host)")
			csUserAgent := getString(string(*msg.Body), "cs(User-Agent)")

			err = d.Incr("request",
				appendTags(
					createTag("club_name", config.Club),
					createTag("c_ip", cIP),
					createTag("time_taken", strconv.FormatFloat(timeTaken, 'G', -1, 32)),
					createTag("cs_uri_stem", csURIStem),
					createTag("x_edge_location", xEdgeLocation),
					createTag("x_edge_result_type", xEdgeResultType),
					createTag("date", date),
					createTag("time", time),
					createTag("cs_method", csMethod),
					createTag("sc_status", scStatus),
					createTag("cs_uri_query", csURIQuery),
					createTag("x_edge_request_id", xEdgeRequestID),
					createTag("x_host_header", xHostHeader),
					createTag("cs_protocol", csProtocol),
					createTag("x_forwarded_for", xff),
					createTag("ssl_protocol", sslProto),
					createTag("ssl_cipher", sslCipher),
					createTag("x_edge_response_result_type", xEdgeResponseResultType),
					createTag("cs_protocol_version", csProtocolVersion),
					createTag("fle_status", fleStatus),
					createTag("fle_encrypted_fields", fleEncryptedFields),
					createTag("cs_host", csHost),
					createTag("cs_user_agent", csUserAgent)),
				1)
			if err != nil {
				log.Printf("datadog request count metric error: %v\n%v", msg, err)
				sendEvent(d, statsd.Event{
					Title:     "datadog metric error",
					Text:      fmt.Sprintf("%s: %v. %v", "datadog request count metric error", msg, err),
					AlertType: statsd.Error,
				})
			}

			// request result type: Miss, Hit and etc per object in cache/file per edge location
			// files that don't exist
			err = d.Incr("result_type",
				appendTags(
					createTag("club_name", config.Club),
					createTag("c_ip", cIP),
					createTag("time_taken", strconv.FormatFloat(timeTaken, 'G', -1, 32)),
					createTag("cs_uri_stem", csURIStem),
					createTag("x_edge_location", xEdgeLocation),
					createTag("x_edge_result_type", xEdgeResultType),
					createTag("date", date),
					createTag("time", time),
					createTag("cs_method", csMethod),
					createTag("sc_status", scStatus),
					createTag("cs_uri_query", csURIQuery),
					createTag("x_edge_request_id", xEdgeRequestID),
					createTag("x_host_header", xHostHeader),
					createTag("cs_protocol", csProtocol),
					createTag("x_forwarded_for", xff),
					createTag("ssl_protocol", sslProto),
					createTag("ssl_cipher", sslCipher),
					createTag("x_edge_response_result_type", xEdgeResponseResultType),
					createTag("cs_protocol_version", csProtocolVersion),
					createTag("fle_status", fleStatus),
					createTag("fle_encrypted_fields", fleEncryptedFields),
					createTag("cs_host", csHost),
					createTag("cs_user_agent", csUserAgent)),
				1)
			if err != nil {
				log.Printf("datadog result_type count metric error: %v\n%v", msg, err)
				sendEvent(d, statsd.Event{
					Title:     "datadog metric error",
					Text:      fmt.Sprintf("%s: %v. %v", "datadog result_type metric error", msg, err),
					AlertType: statsd.Error,
				})
			}

			err = d.Gauge("request_time",
				timeTaken,
				appendTags(
					createTag("club_name", config.Club),
					createTag("c_ip", cIP),
					createTag("cs_uri_stem", csURIStem),
					createTag("x_edge_location", xEdgeLocation),
					createTag("x_edge_result_type", xEdgeResultType),
					createTag("date", date),
					createTag("time", time),
					createTag("cs_method", csMethod),
					createTag("sc_status", scStatus),
					createTag("cs_uri_query", csURIQuery),
					createTag("x_edge_request_id", xEdgeRequestID),
					createTag("x_host_header", xHostHeader),
					createTag("cs_protocol", csProtocol),
					createTag("x_forwarded_for", xff),
					createTag("ssl_protocol", sslProto),
					createTag("ssl_cipher", sslCipher),
					createTag("x_edge_response_result_type", xEdgeResponseResultType),
					createTag("cs_protocol_version", csProtocolVersion),
					createTag("fle_status", fleStatus),
					createTag("fle_encrypted_fields", fleEncryptedFields),
					createTag("cs_host", csHost),
					createTag("cs_user_agent", csUserAgent)),
				1)
			if err != nil {
				log.Printf("datadog request_time gauge metric error: %v\n%v", msg, err)
				sendEvent(d, statsd.Event{
					Title:     "datadog metric error",
					Text:      fmt.Sprintf("%s: %v. %v", "datadog request_time metric error", msg, err),
					AlertType: statsd.Error,
				})
			}
			log.Printf("%s", "process SQS message")
			sendEvent(d, statsd.Event{
				Title: "process SQS message",
				Text:  fmt.Sprintf("%s", "process sqs message successful"),
			})
			deleteMessageStream <- msg.ReceiptHandle
		case <-time.After(time.Duration(config.HeartbeatInterval) * time.Second):
			aliveParser <- "i am alive"
		}
	}
}

func deleteMessage(d *statsd.Client,
	svc *sqs.SQS,
	deleteMessageStream <-chan *string,
	wg *sync.WaitGroup,
	aliveDelete chan<- string) {
	defer wg.Done()
	for {
		select {
		case msg := <-deleteMessageStream:
			params := &sqs.DeleteMessageInput{
				QueueUrl:      &config.SqsQueueURL,
				ReceiptHandle: msg,
			}

			_, err := svc.DeleteMessage(params)
			if err != nil {
				sendEvent(d, statsd.Event{
					Title:     "delete SQS message error",
					Text:      fmt.Sprintf("%v %v", msg, err),
					AlertType: statsd.Error,
				})
				log.Fatalf("delete SQS message error: %v\n%v", msg, err)
			}
		case <-time.After(time.Duration(config.HeartbeatInterval) * time.Second):
			aliveDelete <- "i am alive"
		}
	}
}

func heartbeatParse(d *statsd.Client, aliveParser <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case msg := <-aliveParser:
			sendEvent(d, statsd.Event{
				Title: "heartbeat parser",
				Text:  fmt.Sprintf("%s", msg),
			})
			log.Printf("%s %s", "heartbeat parser", msg)
		case <-time.After(time.Duration(config.HeartbeatTimeout) * time.Second):
			sendEvent(d, statsd.Event{
				Title:     "heartbeat",
				Text:      "parser go routine not healthy",
				AlertType: statsd.Error,
			})
			log.Printf("%s %d", "no heartbeat received from parser with interval", config.HeartbeatInterval)
		}
	}
}

func heartbeatDelete(d *statsd.Client, aliveDelete <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case msg := <-aliveDelete:
			sendEvent(d, statsd.Event{
				Title: "heartbeat delete",
				Text:  fmt.Sprintf("%s", msg),
			})
			log.Printf("%s %s", "heartbeat delete ", msg)
		case <-time.After(time.Duration(config.HeartbeatTimeout) * time.Second):
			sendEvent(d, statsd.Event{
				Title:     "heartbeat",
				Text:      "delete go routine not healthy",
				AlertType: statsd.Error,
			})
			log.Printf("%s %d", "no heartbeat received from delete with interval", config.HeartbeatInterval)
		}
	}
}

func sendEvent(d *statsd.Client, e statsd.Event) {
	// Unfortunately, we can only use Datadog predefined sources. However, if we use a source that is not
	// in this list, Datadog does not drop the event but rather ignore this invalid source name.
	e.SourceTypeName = "apps"
	e.Tags = appendTags(createTag("club_name", config.Club), createTag("app_name", sourceApp))
	err := d.Event(&e)
	if err != nil {
		log.Printf("sendEvent error: %v", err)
	}
}
