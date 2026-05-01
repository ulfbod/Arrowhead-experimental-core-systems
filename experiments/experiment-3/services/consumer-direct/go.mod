module arrowhead/experiment3/consumer-direct

go 1.22

require arrowhead/message-broker v0.0.0

require github.com/rabbitmq/amqp091-go v1.10.0 // indirect

replace arrowhead/message-broker => ../../../../support/message-broker
