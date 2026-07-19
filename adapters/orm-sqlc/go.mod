module github.com/mgo-framework/mgo/adapters/orm-sqlc

go 1.26.5

require github.com/mgo-framework/mgo/adapters/db-sql v0.0.0

require github.com/mgo-framework/mgo/contracts v0.0.0 // indirect

replace (
	github.com/mgo-framework/mgo/adapters/db-sql => ../db-sql
	github.com/mgo-framework/mgo/contracts => ../../contracts
)
