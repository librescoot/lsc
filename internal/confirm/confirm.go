package confirm

import (
	"context"
	"fmt"
	"librescoot/lsc/internal/redis"
	"time"
)

// Subscribe before sending commands, then read immediately: both close the
// Redis pub/sub race while accepting already-satisfied state.
func WaitForFieldValue(ctx context.Context, client *redis.Client, hashKey, field, expectedValue string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pubsub := client.Subscribe(ctx, hashKey)
	defer pubsub.Close()

	ch := pubsub.Channel()

	currentValue, err := client.HGetWithContext(ctx, hashKey, field)
	if err == nil && currentValue == expectedValue {
		return nil
	}

	// Wait for the expected value
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s:%s to become '%s'", hashKey, field, expectedValue)
		case msg := <-ch:
			if msg.Payload == field || msg.Payload == "" {
				currentValue, err := client.HGetWithContext(ctx, hashKey, field)
				if err != nil {
					continue
				}
				if currentValue == expectedValue {
					return nil
				}
			}
		}
	}
}

// Subscribe before executing the command to avoid losing its field notification.
func WaitForFieldValueAfterCommand(ctx context.Context, client *redis.Client, hashKey, field, expectedValue string, timeout time.Duration, commandFunc func() error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pubsub := client.Subscribe(ctx, hashKey)
	defer pubsub.Close()

	ch := pubsub.Channel()

	if err := commandFunc(); err != nil {
		return err
	}

	currentValue, err := client.HGetWithContext(ctx, hashKey, field)
	if err == nil && currentValue == expectedValue {
		return nil
	}

	// Wait for the expected value
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s:%s to become '%s'", hashKey, field, expectedValue)
		case msg := <-ch:
			if msg.Payload == field || msg.Payload == "" {
				currentValue, err := client.HGetWithContext(ctx, hashKey, field)
				if err != nil {
					continue
				}
				if currentValue == expectedValue {
					return nil
				}
			}
		}
	}
}

// WaitForFieldAnyValueAfterCommand uses the same subscribe-first ordering for
// commands with multiple valid terminal states.
func WaitForFieldAnyValueAfterCommand(ctx context.Context, client *redis.Client, hashKey, field string, expectedValues []string, timeout time.Duration, commandFunc func() error) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	match := func(v string) bool {
		for _, ev := range expectedValues {
			if v == ev {
				return true
			}
		}
		return false
	}

	pubsub := client.Subscribe(ctx, hashKey)
	defer pubsub.Close()

	ch := pubsub.Channel()

	if err := commandFunc(); err != nil {
		return "", err
	}

	currentValue, err := client.HGetWithContext(ctx, hashKey, field)
	if err == nil && match(currentValue) {
		return currentValue, nil
	}

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for %s:%s to become one of %v", hashKey, field, expectedValues)
		case msg := <-ch:
			if msg.Payload == field || msg.Payload == "" {
				currentValue, err := client.HGetWithContext(ctx, hashKey, field)
				if err != nil {
					continue
				}
				if match(currentValue) {
					return currentValue, nil
				}
			}
		}
	}
}

func WaitForStateChange(ctx context.Context, client *redis.Client, expectedState string, timeout time.Duration) error {
	return WaitForFieldValue(ctx, client, "vehicle", "state", expectedState, timeout)
}

func WaitForAlarmStatus(ctx context.Context, client *redis.Client, expectedStatus string, timeout time.Duration) error {
	return WaitForFieldValue(ctx, client, "alarm", "status", expectedStatus, timeout)
}
