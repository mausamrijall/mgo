module github.com/mgo-framework/mgo/examples/blog

go 1.26.5

require (
	entgo.io/ent v0.14.1
	github.com/glebarez/sqlite v1.11.0
	github.com/mgo-framework/mgo/adapters/db-sql v0.0.0
	github.com/mgo-framework/mgo/adapters/orm-gorm v0.0.0
	github.com/mgo-framework/mgo/adapters/router-stdmux v0.0.0
	github.com/mgo-framework/mgo/contracts v0.0.0
	github.com/mgo-framework/mgo/framework v0.0.0
	gorm.io/gorm v1.25.12
)

require (
	ariga.io/atlas v0.19.1-0.20240203083654-5948b60a8e43 // indirect
	github.com/agext/levenshtein v1.2.1 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/glebarez/go-sqlite v1.21.2 // indirect
	github.com/go-openapi/inflect v0.19.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/hcl/v2 v2.13.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-wordwrap v0.0.0-20150314170334-ad45545899c7 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/zclconf/go-cty v1.8.0 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	modernc.org/libc v1.55.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/sqlite v1.34.4 // indirect
)

replace (
	github.com/mgo-framework/mgo/adapters/db-sql => ../../adapters/db-sql
	github.com/mgo-framework/mgo/adapters/orm-gorm => ../../adapters/orm-gorm
	github.com/mgo-framework/mgo/adapters/router-stdmux => ../../adapters/router-stdmux
	github.com/mgo-framework/mgo/contracts => ../../contracts
	github.com/mgo-framework/mgo/framework => ../../framework
)
