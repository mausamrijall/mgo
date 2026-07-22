module github.com/mgo-framework/mgo/examples/greeter

go 1.26.5

require (
	github.com/mgo-framework/mgo/adapters/grpc-server v0.0.0
	github.com/mgo-framework/mgo/adapters/router-stdmux v0.0.0
	github.com/mgo-framework/mgo/contracts v0.0.0
	github.com/mgo-framework/mgo/framework v0.0.0
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.10
)

require (
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)

replace (
	github.com/mgo-framework/mgo/adapters/grpc-server => ../../adapters/grpc-server
	github.com/mgo-framework/mgo/adapters/router-stdmux => ../../adapters/router-stdmux
	github.com/mgo-framework/mgo/contracts => ../../contracts
	github.com/mgo-framework/mgo/framework => ../../framework
)
