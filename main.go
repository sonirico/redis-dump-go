package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"sync"

	"github.com/yannh/redis-dump-go/pkg/config"
	"github.com/yannh/redis-dump-go/pkg/redisdump"
)

type progressLogger struct {
	stats map[uint8]int
}

func newProgressLogger() *progressLogger {
	return &progressLogger{
		stats: map[uint8]int{},
	}
}

func (p *progressLogger) drawProgress(to io.Writer, db uint8, nDumped int) {
	if _, ok := p.stats[db]; !ok && len(p.stats) > 0 {
		// We switched database, write to a new line
		fmt.Fprintf(to, "\n")
	}

	p.stats[db] = nDumped
	if nDumped == 0 {
		return
	}

	fmt.Fprintf(to, "\rDatabase %d: %d element dumped", db, nDumped)
}

func realMain() int {
	var err error

	c, outBuf, err := config.FromFlags(os.Args[0], os.Args[1:])
	if outBuf != "" {
		out := os.Stderr
		errCode := 1
		if c.Help {
			out = os.Stdout
			errCode = 0
		}
		fmt.Fprintln(out, outBuf)
		return errCode
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed parsing command line: %s\n", err.Error())
		return 1
	}

	var tlshandler *redisdump.TlsHandler = nil
	if c.Tls == true {
		tlshandler = redisdump.NewTlsHandler(c.CaCert, c.Cert, c.Key)
	}

	var serializer func([]string) string
	switch c.Output {
	case "resp":
		serializer = redisdump.RESPSerializer

	case "commands":
		serializer = redisdump.RedisCmdSerializer

	default:
		log.Fatalf("Failed parsing parameter flag: can only be resp or json")
	}

	redisPassword := os.Getenv("REDISDUMPGO_AUTH")

	progressNotifs := make(chan redisdump.ProgressNotification)
	var wg sync.WaitGroup
	wg.Add(1)

	defer func() {
		close(progressNotifs)
		wg.Wait()
		if !(c.Silent) {
			fmt.Fprint(os.Stderr, "\n")
		}
	}()

	pl := newProgressLogger()
	go func() {
		for n := range progressNotifs {
			if !(c.Silent) {
				pl.drawProgress(os.Stderr, n.Db, n.Done)
			}
		}
		wg.Done()
	}()

	logger := log.New(os.Stdout, "", 0)
	if c.Db < 0 { // <0 is for all dbs, easier to manage than a *uint unfortunately
		if err = redisdump.DumpServer(c.Host, c.Port, url.QueryEscape(redisPassword), tlshandler, c.Filter, c.NWorkers, c.WithTTL, c.BatchSize, c.Noscan, logger, serializer, progressNotifs); err != nil {
			fmt.Fprintf(os.Stderr, "%s", err)
			return 1
		}
	} else {
		var db *uint8 = new(uint8)
		if c.Db != -1 {
			*db = uint8(c.Db)
		}
		if err = redisdump.DumpDB(c.Host, c.Port, url.QueryEscape(redisPassword), tlshandler, db, c.Filter, c.NWorkers, c.WithTTL, c.BatchSize, c.Noscan, logger, serializer, progressNotifs); err != nil {
			fmt.Fprintf(os.Stderr, "%s", err)
			return 1
		}
	}
	return 0
}

func main() {
	os.Exit(realMain())
}
