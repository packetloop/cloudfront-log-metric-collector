cloudfront-log-metric-generator
===============================

Due to Cloudfront integration in Datadog does not give us what we need such as
number of requests per edge location. Hence, we parse Cloudfront logs and
convert these logs to metrics to allow us to measure and monitor our Cloudfront
service.

This does not get logs from S3 buckets ( where Cloudfront logs stores its logs).
But instead, this receives messages from SQS topic. There are several ways to
retrieve logs from S3 bucket and write to a SQS topic. For instance, we use
fluent-plugin-cloudfront-logs ( https://github.com/kubihie/fluent-plugin-cloudfront-log )
for input and fluent-plugin-sqs ( https://github.com/ixixi/fluent-plugin-sqs ) to
write these logs to SQS.

Usage:
------

To compile and run binary:

```bash
$ SQS_REGION=ap-southeast-2 GOROUTINE=1 \
    SQS_QUEUE_URL=https://sqs.ap-southeast-2.amazonaws.com/143032791481/testqueue \
    STATSD_HOST=127.0.0.1:8125 \
    CLUB_NAME=dev \
	make run
```

To compile:

```bash
$ make compile
```

To release a new version:

```bash
$ make release TAG=<maj.minor.patch>
```

To run tests:

```bash
$ make test
```

To do a release and ci build: ( if you're executing this from
your machine, please make sure you have docker-machine up and
env exported properly).

```bash
$ make ci-build
```
