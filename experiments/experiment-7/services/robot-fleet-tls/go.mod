module arrowhead/experiment7/robot-fleet-tls

go 1.22.0

require (
	arrowhead/message-broker v0.0.0
	github.com/segmentio/kafka-go v0.4.47
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
)

replace arrowhead/message-broker => ../../../../support/message-broker
