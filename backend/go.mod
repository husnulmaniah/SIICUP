module cuti-app

go 1.24.7

replace (
	golang.org/x/arch => github.com/golang/arch v0.8.0
	golang.org/x/crypto => github.com/golang/crypto v0.23.0
	golang.org/x/net => github.com/golang/net v0.25.0
	golang.org/x/sync => github.com/golang/sync v0.7.0
	golang.org/x/sys => github.com/golang/sys v0.20.0
	golang.org/x/term => github.com/golang/term v0.20.0
	golang.org/x/text => github.com/golang/text v0.15.0
	golang.org/x/tools => github.com/golang/tools v0.21.0
	google.golang.org/protobuf => github.com/protocolbuffers/protobuf-go v1.34.1
	gopkg.in/check.v1 => github.com/go-check/check v0.0.0-20201130134442-10cb98267c6c
	gopkg.in/yaml.v3 => github.com/go-yaml/yaml v3.0.1+incompatible
	gorm.io/driver/postgres => github.com/go-gorm/postgres v1.5.9
	gorm.io/gorm => github.com/go-gorm/gorm v1.25.12
)

require (
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/joho/godotenv v1.5.1
	github.com/xuri/excelize/v2 v2.9.0
	golang.org/x/crypto v0.28.0
	gorm.io/driver/postgres v1.5.9
	gorm.io/gorm v1.25.12
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.5.5 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.4 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/xuri/efp v0.0.0-20240408161823-9ad904a10d6d // indirect
	github.com/xuri/nfp v0.0.0-20240318013403-ab9948c2c4a7 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/text v0.19.0 // indirect
)
