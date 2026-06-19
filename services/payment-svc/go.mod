module github.com/sanusi/banking/services/payment-svc

go 1.26.3

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-playground/validator/v10 v10.22.0
	github.com/golang-migrate/migrate/v4 v4.19.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.5.5
	github.com/nats-io/nats.go v1.37.0
	github.com/prometheus/client_golang v1.23.2
	github.com/redis/go-redis/v9 v9.19.0
	github.com/sanusi/banking/pkg v0.0.0
	go.opentelemetry.io/otel v1.43.0
	gorm.io/gorm v1.25.11
)

replace github.com/sanusi/banking/pkg => ../../pkg
