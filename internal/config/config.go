package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// For aggregation over multiple values at different cpus/sockets/..., not time!
type AggregationStrategy int

const (
	NoAggregation AggregationStrategy = iota
	SumAggregation
	AvgAggregation
)

func (as *AggregationStrategy) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	switch str {
	case "":
		*as = NoAggregation
	case "sum":
		*as = SumAggregation
	case "avg":
		*as = AvgAggregation
	default:
		return fmt.Errorf("invalid aggregation strategy: %#v", str)
	}
	return nil
}

type MetricConfig struct {
	// Interval in seconds at which measurements will arive.
	Frequency int64 `json:"frequency"`

	// Can be 'sum', 'avg' or null. Describes how to aggregate metrics from the same timestep over the hierarchy.
	Aggregation AggregationStrategy `json:"aggregation"`

	// Private, used internally...
	Offset int
}

type HttpConfig struct {
	// Address to bind to, for example "0.0.0.0:8081"
	Address string `json:"address"`

	// If not the empty string, use https with this as the certificate file
	CertFile string `json:"https-cert-file"`

	// If not the empty string, use https with this as the key file
	KeyFile string `json:"https-key-file"`
}

type NatsConfig struct {
	// Address of the nats server
	Address string `json:"address"`

	// Username/Password, optional
	Username string `json:"username"`
	Password string `json:"password"`

	Subscriptions []struct {
		// Channel name
		SubscribeTo string `json:"subscribe-to"`

		// Allow lines without a cluster tag, use this as default, optional
		ClusterTag string `json:"cluster-tag"`
	} `json:"subscriptions"`
}

type Config struct {
	Metrics     map[string]MetricConfig `json:"metrics"`
	HttpConfig  *HttpConfig             `json:"http-api"`
	Checkpoints struct {
		Interval string `json:"interval"`
		RootDir  string `json:"directory"`
		Restore  string `json:"restore"`
	} `json:"checkpoints"`
	Debug struct {
		DumpToFile string `json:"dump-to-file"`
		EnableGops bool   `json:"gops"`
	} `json:"debug"`
	RetentionInMemory string `json:"retention-in-memory"`
	JwtPublicKey      string `json:"jwt-public-key"`
	Archive           struct {
		Interval      string `json:"interval"`
		RootDir       string `json:"directory"`
		DeleteInstead bool   `json:"delete-instead"`
	} `json:"archive"`
	Nats []*NatsConfig `json:"nats"`
}

func LoadConfiguration(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer configFile.Close()
	dec := json.NewDecoder(configFile)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&config); err != nil {
		log.Fatal(err)
	}
	return config
}
