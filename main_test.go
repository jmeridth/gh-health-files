package main

import (
	"testing"
	"time"

	"github.com/google/go-github/v53/github"
	"github.com/stretchr/testify/assert"
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

func TestRateLimitCheck(t *testing.T) {
	resp := &github.Response{
		Rate: github.Rate{
			Remaining: 0,
			Reset:     github.Timestamp{Time: time.Now().Add(2 * time.Second)},
		},
	}

	start := time.Now()
	rateLimitCheck(resp)
	duration := time.Since(start)

	assert.GreaterOrEqual(t, duration, 2*time.Second, "Expected rate limit check to wait for at least 1 minute")
}
