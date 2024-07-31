package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
)

var (
	SubscriptionCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subscriptions_total",
			Help: "Total number of subscriptions.",
		},
		[]string{"status"},
	)
	NotificationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_total",
			Help: "Total number of notifications sent.",
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(SubscriptionCounter)
	prometheus.MustRegister(NotificationCounter)
}

func StartMetricsServer() {
	http.Handle("/metrics", promhttp.Handler())
	log.Println("Metrics server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
