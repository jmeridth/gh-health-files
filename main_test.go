package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/v72/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateFileNameVariations(t *testing.T) {
	fileName := "CODE_OF_CONDUCT.md"
	expected := []string{
		"CODE_OF_CONDUCT.md",
		"CODE_OF_CONDUCT.MD",
		"code_of_conduct.md",
		"CODE-OF-CONDUCT.md",
		"code-of-conduct.md",
		"Code_Of_Conduct.md",
	}

	variations := generateFileNameVariations(fileName)
	assert.ElementsMatch(t, expected, variations, "File name variations do not match expected values")
}

func TestRateLimitWait(t *testing.T) {
	t.Run("waits until reset", func(t *testing.T) {
		resp := &github.Response{
			Rate: github.Rate{
				Remaining: 0,
				Reset:     github.Timestamp{Time: time.Now().Add(2 * time.Second)},
			},
		}

		start := time.Now()
		err := rateLimitWait(context.Background(), resp)
		duration := time.Since(start)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, duration, 2*time.Second, "Expected rate limit wait of at least 2 seconds")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		resp := &github.Response{
			Rate: github.Rate{
				Remaining: 0,
				Reset:     github.Timestamp{Time: time.Now().Add(10 * time.Second)},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := rateLimitWait(ctx, resp)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("no-op when remaining > 0", func(t *testing.T) {
		resp := &github.Response{
			Rate: github.Rate{
				Remaining: 100,
			},
		}

		err := rateLimitWait(context.Background(), resp)
		require.NoError(t, err)
	})

	t.Run("no-op when resp is nil", func(t *testing.T) {
		err := rateLimitWait(context.Background(), nil)
		require.NoError(t, err)
	})
}
