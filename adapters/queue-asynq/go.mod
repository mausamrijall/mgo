module github.com/mgo-framework/mgo/adapters/queue-asynq

go 1.26.5

require (
	github.com/hibiken/asynq v0.25.1
	github.com/mgo-framework/mgo/contracts v0.0.0
)

require github.com/alicebob/miniredis/v2 v2.34.0

require (
	github.com/alicebob/gopher-json v0.0.0-20230218143504-906a9b012302 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/redis/go-redis/v9 v9.7.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	google.golang.org/protobuf v1.35.2 // indirect
)

replace github.com/mgo-framework/mgo/contracts => ../../contracts
