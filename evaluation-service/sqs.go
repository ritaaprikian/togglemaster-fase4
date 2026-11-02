package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// traceAttributes copia o contexto do trace para os atributos da mensagem,
// permitindo que o analytics-service continue o mesmo trace ao consumir a fila.
func traceAttributes(ctx context.Context) map[string]*sqs.MessageAttributeValue {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	attributes := make(map[string]*sqs.MessageAttributeValue, len(carrier))
	for key, value := range carrier {
		attributes[key] = &sqs.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(value),
		}
	}
	return attributes
}

// Evento que será enviado para a fila
type EvaluationEvent struct {
	UserID    string    `json:"user_id"`
	FlagName  string    `json:"flag_name"`
	Result    bool      `json:"result"`
	Timestamp time.Time `json:"timestamp"`
}

// sendEvaluationEvent envia um evento para a fila SQS
func (a *App) sendEvaluationEvent(ctx context.Context, userID, flagName string, result bool) {
	// Se a URL da fila não foi configurada, apenas loga localmente e sai.
	if a.SqsSvc == nil || a.SqsQueueURL == "" {
		log.Printf("[SQS_DISABLED] Evento: User '%s', Flag '%s', Result '%t'", userID, flagName, result)
		return
	}

	event := EvaluationEvent{
		UserID:    userID,
		FlagName:  flagName,
		Result:    result,
		Timestamp: time.Now().UTC(),
	}

	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("Erro ao serializar evento SQS: %v", err)
		return
	}

	spanCtx, span := tracer.Start(ctx, "togglemaster-events publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "aws_sqs"),
			attribute.String("messaging.destination.name", "togglemaster-events"),
		),
	)
	defer span.End()

	// Envia a mensagem
	_, err = a.SqsSvc.SendMessageWithContext(spanCtx, &sqs.SendMessageInput{
		MessageBody:       aws.String(string(body)),
		QueueUrl:          aws.String(a.SqsQueueURL),
		MessageAttributes: traceAttributes(spanCtx),
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "falha ao publicar na fila")
		log.Printf("Erro ao enviar mensagem para SQS: %v", err)
	} else {
		log.Printf("Evento de avaliação enviado para SQS (Flag: %s)", flagName)
	}
}
