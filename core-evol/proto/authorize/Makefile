# Regenerate Go code from authorize.proto.
#
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc
#
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.4.0

.PHONY: gen clean

gen:
	protoc \
	  --go_out=. \
	  --go_opt=paths=source_relative \
	  --go-grpc_out=. \
	  --go-grpc_opt=paths=source_relative \
	  authorize.proto

clean:
	rm -f authorize.pb.go authorize_grpc.pb.go
