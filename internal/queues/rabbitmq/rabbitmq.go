package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

type Queue struct {
	queue  *amqp091.Queue
	conn   *amqp091.Connection
	ch     *amqp091.Channel
	logger *logrus.Logger
}
type QueueConfig struct {
	Name        string
	Durable     bool // queue survives restart
	DeleteOnUse bool // autoDelete: don't delete when consumers disconnect
	Exclusive   bool // can be accessed by other connections
	NoWait      bool // wait for confirmation
}

func Connect(ctx context.Context, url string, logger *logrus.Logger) (*Queue, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			conn, err := amqp091.Dial(url)

			if err != nil {
				logger.WithError(err).Errorf("Error connecting to RabbitMQ, retrying in 5s...")

				select {
				case <-time.After(5 * time.Second):
				case <-ctx.Done():
					return nil, ctx.Err()
				}

				continue
			}

			ch, err := conn.Channel()
			if err != nil {
				conn.Close()
				logger.WithError(err).Errorln("Error opening RabbitMQ channel")

				continue
			}

			return &Queue{conn: conn, ch: ch, logger: logger}, nil
		}
	}
}

func (q *Queue) CreateQueue(config *QueueConfig) error {
	if q.ch.IsClosed() {
		return errors.New("Cannot use a closed channel")
	}

	queue, err := q.ch.QueueDeclare(
		config.Name,
		config.Durable,
		config.DeleteOnUse,
		config.Exclusive,
		config.NoWait,
		amqp091.Table{
			amqp091.QueueTypeArg: amqp091.QueueTypeQuorum,
		},
	)

	if err != nil {
		q.logger.WithError(err).Errorln("Error opening rabitmq channel")
		return err
	}

	q.queue = &queue
	return nil
}

func (q *Queue) AddToQueue(j any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := json.Marshal(j)

	if err != nil {
		q.logger.WithError(err).Errorln("Error marshaling json for queueing")
		return err
	}

	err = q.ch.PublishWithContext(ctx,
		"",           // exchange
		q.queue.Name, // routing key
		false,        // mandatory
		false,
		amqp091.Publishing{
			DeliveryMode: amqp091.Persistent,
			ContentType:  "application/json",
			Body:         body,
		})

	if err != nil {
		q.logger.WithError(err).Errorln("Error opening rabitmq channel")
		return err
	}

	q.logger.Info("Job added to queue")

	return nil
}

func (q *Queue) Close() {
	if q.ch != nil {
		q.ch.Close()
	}

	if q.conn != nil {
		q.conn.Close()
	}
}
