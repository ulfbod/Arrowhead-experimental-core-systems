# Makefile — regenerate Go bindings from certlifecycle.proto.
# Uses Docker so no local protoc installation is required.
# Usage: make gen

PROTO_FILE := certlifecycle.proto
PROTO_GO   := certlifecycle.pb.go certlifecycle_grpc.pb.go

.PHONY: gen clean

gen: $(PROTO_GO)

$(PROTO_GO): $(PROTO_FILE)
	docker run --rm \
	  -v "$(CURDIR):/proto" \
	  golang:1.22-alpine sh -c " \
	    apk add --no-cache protobuf protobuf-dev 2>/dev/null; \
	    go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2; \
	    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.4.0; \
	    protoc \
	      --go_out=/proto --go_opt=paths=source_relative \
	      --go-grpc_out=/proto --go-grpc_opt=paths=source_relative \
	      -I /proto /proto/$(PROTO_FILE) \
	  "

clean:
	rm -f $(PROTO_GO)
