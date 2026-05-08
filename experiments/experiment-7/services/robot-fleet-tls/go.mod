module arrowhead/experiment7/robot-fleet-tls

go 1.22.0

require (
	arrowhead/message-broker v0.0.0
	github.com/segmentio/kafka-go v0.4.47
)

replace arrowhead/message-broker => ../../../../support/message-broker
