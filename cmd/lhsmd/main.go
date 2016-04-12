package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/rcrowley/go-metrics"
	"github.com/vrischmann/go-metrics-influxdb"
	"github.intel.com/hpdd/logging/alert"
	"github.intel.com/hpdd/logging/audit"
	"github.intel.com/hpdd/logging/debug"
	"github.intel.com/hpdd/policy/pdm/lhsmd/agent"

	// Register the supported transports
	_ "github.intel.com/hpdd/policy/pdm/lhsmd/transport/grpc"
	//_ "github.intel.com/hpdd/policy/pdm/lhsmd/transport/queue"
)

func init() {
	flag.Var(debug.FlagVar())
}

func interruptHandler(once func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		stopping := false
		for sig := range c {
			debug.Printf("signal received: %s", sig)
			if !stopping {
				stopping = true
				once()
			}
		}
	}()

}

func main() {
	flag.Parse()

	if debug.Enabled() {
		// Set this so that plugins can use it without needing
		// to mess around with plugin args.
		os.Setenv(debug.EnableEnvVar, "true")
	}

	// Setting the prefix helps us to track down deprecated calls to log.*
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(audit.Writer().Prefix("DEPRECATED "))

	conf := agent.ConfigInitMust()

	debug.Printf("current configuration:\n%v", conf.String())
	if err := configureMounts(conf); err != nil {
		alert.Fatalf("Error while creating Lustre mountpoints: %s", err)
	}

	ct, err := agent.New(conf)
	if err != nil {
		alert.Fatalf("Error creating agent: %s", err)
	}

	if conf.InfluxURL != "" {
		go influxdb.InfluxDB(
			metrics.DefaultRegistry, // metrics registry
			time.Second*10,          // interval
			conf.InfluxURL,
			conf.InfluxDB,       // your InfluxDB database
			conf.InfluxUser,     // your InfluxDB user
			conf.InfluxPassword, // your InfluxDB password
		)
	}

	ctx, cancel := context.WithCancel(context.Background())
	interruptHandler(func() {
		ct.Stop()
		cancel()
	})

	if err := ct.Start(ctx); err != nil {
		alert.Fatalf("Error in HsmAgent.Start(): %s", err)
	}
}
