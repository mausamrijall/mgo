module github.com/mgo-framework/mgo/examples/hello

go 1.26.5

require (
	github.com/mgo-framework/mgo/contracts v0.0.0
	github.com/mgo-framework/mgo/framework v0.0.0
)

replace github.com/mgo-framework/mgo/contracts => ../../contracts

replace github.com/mgo-framework/mgo/framework => ../../framework
