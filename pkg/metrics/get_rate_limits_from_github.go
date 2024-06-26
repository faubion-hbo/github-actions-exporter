package metrics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	remainingLimitsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_remaining_limits",
			Help: "remaining limits",
		},
		[]string{"app_id", "type"},
	)
)

var rateLimitsQuerySecondsBetween = 5

func getRateLimits() (*github.RateLimits, int) {
	rateLimits, resp, err := client.RateLimits(context.Background())

	if rl_err, ok := err.(*github.RateLimitError); ok {
		resetTime := rl_err.Rate.Reset.Time
		delaySeconds := int(time.Until(resetTime).Seconds())
		log.Printf("RateLimits ratelimited, sleeping for %ds (at %s)", delaySeconds, resetTime.String())
		return nil, delaySeconds
	} else if err != nil {
		if resp != nil && resp.StatusCode == http.StatusForbidden {
			if retryAfterSeconds, parseErr := strconv.ParseInt(resp.Header.Get("Retry-After"), 10, 32); parseErr == nil {
				delaySeconds := int(retryAfterSeconds + (60 * rand.Int63n(randomDelaySeconds)))
				log.Printf("RateLimits Retry-After %d seconds received, sleeping for %ds", retryAfterSeconds, delaySeconds)
				return nil, delaySeconds
			}
		}
		log.Printf("RateLimits error: %s", err.Error())
		return nil, rateLimitsQuerySecondsBetween
	}

	return rateLimits, rateLimitsQuerySecondsBetween
}

// getRemainingLimitsFromGithub - return information about the remaining limits for this GitHub client's credentials
func getRemainingLimitsFromGithub(appId string) {
	appIdBytes := []byte(appId)
	hashedAppIdBytes := sha256.New()
	hashedAppIdBytes.Write(appIdBytes)
	hashedAppId := fmt.Sprintf("%x", hashedAppIdBytes.Sum(nil))

	for {
		rateLimits, secondsBetween := getRateLimits()

		if rateLimits != nil {
			remainingLimitsGauge.WithLabelValues(hashedAppId, "core").Set(float64(rateLimits.GetCore().Remaining))
			remainingLimitsGauge.WithLabelValues(hashedAppId, "search").Set(float64(rateLimits.GetSearch().Remaining))
			remainingLimitsGauge.WithLabelValues(hashedAppId, "graphql").Set(float64(rateLimits.GetGraphQL().Remaining))
		}

		time.Sleep(time.Duration(secondsBetween) * time.Second)
	}
}
