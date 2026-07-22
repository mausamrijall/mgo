module github.com/mgo-framework/mgo/examples/authdemo

go 1.26.5

require (
	github.com/mgo-framework/mgo/adapters/auth-jwt v0.0.0
	github.com/mgo-framework/mgo/adapters/hash-argon2 v0.0.0
	github.com/mgo-framework/mgo/adapters/router-chi v0.0.0
	github.com/mgo-framework/mgo/adapters/router-stdmux v0.0.0
	github.com/mgo-framework/mgo/adapters/session-scs v0.0.0
	github.com/mgo-framework/mgo/contracts v0.0.0
	github.com/mgo-framework/mgo/framework v0.0.0
)

require (
	github.com/alexedwards/scs/v2 v2.8.0 // indirect
	github.com/go-chi/chi/v5 v5.2.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

replace (
	github.com/mgo-framework/mgo/adapters/auth-jwt => ../../adapters/auth-jwt
	github.com/mgo-framework/mgo/adapters/hash-argon2 => ../../adapters/hash-argon2
	github.com/mgo-framework/mgo/adapters/router-chi => ../../adapters/router-chi
	github.com/mgo-framework/mgo/adapters/router-stdmux => ../../adapters/router-stdmux
	github.com/mgo-framework/mgo/adapters/session-scs => ../../adapters/session-scs
	github.com/mgo-framework/mgo/contracts => ../../contracts
	github.com/mgo-framework/mgo/framework => ../../framework
)
