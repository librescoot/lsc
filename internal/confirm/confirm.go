package confirm

import (
	"context"
	"fmt"
	"librescoot/lsc/internal/redis"
	"time"
)

// WaitForFieldValue waits for a Redis hash field to match an expected value
// by subscribing to the channel and checking the field value.
// This function subscribes FIRST before the caller sends commands, to avoid missing notifications.
func WaitForFieldValue(ctx context.Context, client *redis.Client, hashKey, field, expectedValue string, timeout time.Duration) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Subscribe to the hash channel (e.g., "vehicle" for "vehicle" hash)
	// Services publish to this channel when they update the hash
	// IMPORTANT: We subscribe BEFORE the command is sent to avoid race conditions
	pubsub := client.Subscribe(ctx, hashKey)
	defer pubsub.Close()

	// Channel to receive pub/sub messages
	ch := pubsub.Channel()

	// Also check immediately in case the value is already set
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
			// Message payload is typically the field name that changed
			// Check if it's the field we're interested in
			if msg.Payload == field || msg.Payload == "" {
				// Re-check the value
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

// WaitForFieldValueAfterCommand sets up waiting for a field value change, then executes the command function.
// This ensures the subscription is established BEFORE the command is sent, avoiding race conditions.
func WaitForFieldValueAfterCommand(ctx context.Context, client *redis.Client, hashKey, field, expectedValue string, timeout time.Duration, commandFunc func() error) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Subscribe FIRST to avoid missing the notification
	pubsub := client.Subscribe(ctx, hashKey)
	defer pubsub.Close()

	ch := pubsub.Channel()

	// Execute the command after subscription is established
	if err := commandFunc(); err != nil {
		return err
	}

	// Check immediately in case the value is already set
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

// WaitForStateChange waits for vehicle state to change to expected value
func WaitForStateChange(ctx context.Context, client *redis.Client, expectedState string, timeout time.Duration) error {
	return WaitForFieldValue(ctx, client, "vehicle", "state", expectedState, timeout)
}

// WaitForAlarmStatus waits for alarm status to change to expected value
func WaitForAlarmStatus(ctx context.Context, client *redis.Client, expectedStatus string, timeout time.Duration) error {
	return WaitForFieldValue(ctx, client, "alarm", "status", expectedStatus, timeout)
}
