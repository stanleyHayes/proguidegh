// Command migrate runs the versioned SQL migrations embedded in
// internal/migrations. Usage:
//
//	migrate up            apply all pending migrations
//	migrate down [-all]   roll back the latest migration (or all with -all)
//	migrate status        list migrations and their applied state
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"proguidegh/api/internal/migrations"
	"proguidegh/api/internal/platform/config"
	"proguidegh/api/internal/platform/db"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "up":
		done, err := migrations.Up(ctx, pool)
		exitOnErr(err)
		if len(done) == 0 {
			fmt.Println("nothing to apply")
			return
		}
		for _, v := range done {
			fmt.Printf("applied %04d\n", v)
		}

	case "down":
		fs := flag.NewFlagSet("down", flag.ExitOnError)
		all := fs.Bool("all", false, "roll back all applied migrations")
		_ = fs.Parse(os.Args[2:])
		done, err := migrations.Down(ctx, pool, *all)
		exitOnErr(err)
		if len(done) == 0 {
			fmt.Println("nothing to roll back")
			return
		}
		for _, v := range done {
			fmt.Printf("reverted %04d\n", v)
		}

	case "status":
		statuses, err := migrations.Statuses(ctx, pool)
		exitOnErr(err)
		for _, s := range statuses {
			state := "pending"
			if s.Applied {
				state = "applied"
			}
			fmt.Printf("%04d_%s\t%s\n", s.Migration.Version, s.Migration.Name, state)
		}

	default:
		usage()
		os.Exit(2)
	}
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: migrate <command>

commands:
  up            apply all pending migrations
  down [-all]   roll back the latest migration (or all with -all)
  status        list migrations and their applied state`)
}
