package app

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/core-go/health"
	hm "github.com/core-go/health/mongo"
	hr "github.com/core-go/health/rabbitmq"
	w "github.com/core-go/mongo/writer"
	"github.com/core-go/mq"
	v "github.com/core-go/mq/validator"
	"github.com/core-go/mq/zap"
	"github.com/core-go/rabbitmq"
)

type ApplicationContext struct {
	HealthHandler *health.Handler
	Consume       func(ctx context.Context, handle func(context.Context, []byte, map[string]string))
	Handle        func(context.Context, []byte, map[string]string)
}

func NewApp(ctx context.Context, cfg Config) (*ApplicationContext, error) {
	log.Initialize(cfg.Log)
	client, er1 := mongo.Connect(ctx, options.Client().ApplyURI(cfg.Mongo.Uri))
	if er1 != nil {
		log.Error(ctx, "Cannot connect to MongoDB: Error: "+er1.Error())
		return nil, er1
	}
	db := client.Database(cfg.Mongo.Database)

	logError := log.ErrorMsg
	var logInfo func(context.Context, string)
	if log.IsInfoEnable() {
		logInfo = log.InfoMsg
	}

	consumer, er2 := rabbitmq.NewConsumerByConfig(cfg.Consumer, true, true, logError)
	if er2 != nil {
		log.Error(ctx, "Cannot create a new consumer. Error: "+er2.Error())
		return nil, er2
	}
	validator, er3 := v.NewValidator[*User]()
	if er3 != nil {
		return nil, er3
	}
	errorHandler := mq.NewErrorHandler[*User](logError)
	publisher, er4 := rabbitmq.NewPublisherByConfig(*cfg.Publisher)
	if er4 != nil {
		return nil, er4
	}
	writer := w.NewWriter[*User](db, "user")
	handler := mq.NewRetryHandlerByConfig[User](cfg.Retry, writer.Write, validator.Validate, errorHandler.RejectWithMap, nil, publisher.Publish, logError, logInfo)
	mongoChecker := hm.NewHealthChecker(client)
	consumerChecker := hr.NewHealthChecker(cfg.Consumer.Url, "rabbitmq_consumer")
	publisherChecker := hr.NewHealthChecker(cfg.Publisher.Url, "rabbitmq_publisher")
	healthHandler := health.NewHandler(mongoChecker, consumerChecker, publisherChecker)

	return &ApplicationContext{
		HealthHandler: healthHandler,
		Consume:       consumer.Consume,
		Handle:        handler.Handle,
	}, nil
}
