package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

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
const Version = "0.0.1"

var config struct {
	GoRoutine                int    `env:"GoRoutine,default=3"`
	Club                     string `env:"CLUB_NAME,required"`
	SqsRegion                string `env:"SQS_REGION,default=us-west-2"`
	SqsQueueURL              string `env:"SQS_QUEUE_URL,required"`
	SqsWaitTimeSeconds       int64  `env:"SQS_WAIT_TIME_SECONDS,default=10"`
	SqsMaxNumberOfMessages   int64  `env:"SQS_MAX_NUMBER_OF_MESSAGES,default=10"`
	SqsVisibilityTimeout     int64  `env:"SQS_VISIBILITY_TIMEOUT,default=300"`
	SqsMessageAttributeNames string `env:"SQS_MESSAGE_ATTRIBUTE_NAME,default=cloudfront"`
	// StatsdHost format host:port. Eg. 127.0.0.1:8125
	// Only supports UDP since we rely on dogstatsd/datadog agent config.
	StatsdHost   string `env:"STATSD_HOST,required"`
	StatsdPrefix string `env:"STATSD_PREFIX,default=cloudfront."`
}

type queue struct {
	URL     string
	Message *sqs.Message
}

var (
	queueURL = ""
)

func init() {
	if err := envdecode.Decode(&config); err != nil {
		log.Printf("%s\n", err.Error())
		os.Exit(1)
	}
	// We need at least 3 GoRoutines because we fire up 3 goroutines. One for
	// receiving SQS message, parse SQS/Cloudfront logs and delete message from
	// SQS.
	if config.GoRoutine < 3 {
		log.Printf("%s\n", "Pls specify more than 3 GoRoutines. 3 minimum goroutines is required to run this.")
		os.Exit(1)
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

	messageStreamInput := make(chan *sqs.Message, config.SqsMaxNumberOfMessages)
	deleteMessageStream := make(chan *string, config.SqsMaxNumberOfMessages)
	for i := 0; i <= config.GoRoutine; i++ {
		wg.Add(i)
		go receiveMessage(svc, messageStreamInput, &wg)
		go printMessage(m, messageStreamInput, deleteMessageStream, &wg)
		go deleteMessage(svc, deleteMessageStream, &wg)
	}
	wg.Wait()
	log.Printf("%s\n", "cloudfront metric generator finished")
}

func receiveMessage(svc *sqs.SQS, messageStreamInput chan *sqs.Message, wg *sync.WaitGroup) {
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
			panic(err)
		}

		//fmt.Println(resp.GoString())
		for _, i := range resp.Messages {
			messageStreamInput <- i
		}
	}
}

func printMessage(d *statsd.Client, messageStreamInput <-chan *sqs.Message, deleteMessageStream chan<- *string, wg *sync.WaitGroup) {

	/*
		amount of users by region
		length of time connect per unique IP
		geo info out of cloudfront
		files that don't exist
	*/

	wg.Done()
	for {
		select {
		case msg := <-messageStreamInput:

			clientip := gjson.GetBytes([]byte(string(*msg.Body)), "c-ip")
			timeTaken := gjson.GetBytes([]byte(string(*msg.Body)), "time-taken")
			cacheObject := gjson.GetBytes([]byte(string(*msg.Body)), "cs-uri-stem")
			edgeLocation := gjson.GetBytes([]byte(string(*msg.Body)), "x-edge-location")
			result := gjson.GetBytes([]byte(string(*msg.Body)), "x-edge-result-type")
			s, _ := strconv.ParseFloat(fmt.Sprintf("%s", timeTaken), 10)
			//			requestsCount, _ := strconv.Atoi()

			var err error
			// request per region
			// this would give us a metric amount of users per region
			err = d.Incr("request", []string{"testedge:" + edgeLocation.String(), "testenv:dev,"}, 1)
			if err != nil {
				fmt.Printf("Error %v", err)
			}

			// request result type: Miss, Hit and etc per object in cache/file per edge location
			// files that don't exist
			err = d.Incr("result_type", []string{"testedge:" + edgeLocation.String(),
				"testfile:" + cacheObject.String(),
				"test-type:" + result.String(),
				"testenv:dev,"}, 1)
			if err != nil {
				fmt.Printf("Error %v", err)
			}

			//length of time connect per unique IP
			err = d.Gauge("request_time", s, []string{"testfile:" + fmt.Sprintf("%s", cacheObject),
				"testedge:" + edgeLocation.String(),
				"client_ip:" + clientip.String(),
				"testenv:dev"}, 1)
			if err != nil {
				fmt.Printf("Error %v", err)
			}
			deleteMessageStream <- msg.ReceiptHandle
		default:
		}
	}
}

func deleteMessage(svc *sqs.SQS, deleteMessageStream <-chan *string, wg *sync.WaitGroup) {
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
				panic(err)
			}
		default:
		}
	}
}
