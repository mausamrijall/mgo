module github.com/mgo-framework/mgo/adapters/sched-gocron

go 1.26.5

require (
	github.com/go-co-op/gocron/v2 v2.14.0
	github.com/mgo-framework/mgo/contracts v0.0.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	golang.org/x/exp v0.0.0-20240613232115-7f521ea00fb8 // indirect
)

replace github.com/mgo-framework/mgo/contracts => ../../contracts
