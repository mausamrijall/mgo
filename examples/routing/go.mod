module github.com/mgo-framework/mgo/examples/routing

go 1.26.5

require (
	github.com/mgo-framework/mgo/adapters/middleware-cors v0.0.0
	github.com/mgo-framework/mgo/adapters/router-chi v0.0.0
	github.com/mgo-framework/mgo/adapters/router-stdmux v0.0.0
	github.com/mgo-framework/mgo/contracts v0.0.0
	github.com/mgo-framework/mgo/framework v0.0.0
)

require (
	github.com/go-chi/chi/v5 v5.2.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
)

replace (
	github.com/mgo-framework/mgo/adapters/middleware-cors => ../../adapters/middleware-cors
	github.com/mgo-framework/mgo/adapters/router-chi => ../../adapters/router-chi
	github.com/mgo-framework/mgo/adapters/router-stdmux => ../../adapters/router-stdmux
	github.com/mgo-framework/mgo/contracts => ../../contracts
	github.com/mgo-framework/mgo/framework => ../../framework
)
