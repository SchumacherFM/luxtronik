package main

import (
	"log"
	"os"

	"github.com/SchumacherFM/luxtronik"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:   "calculations",
				Usage:  "Starts an HTTP server and shows all changed data",
				Flags:  []cli.Flag{},
				Action: runHTTP,
			},
		},
		Usage: "Luxtronik Viewer",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:     "ip-port",
				Required: false,
				Usage:    "192.168.0.121" + ":" + luxtronik.DefaultPort,
				EnvVars:  []string{"HEATPUMP_IP"},
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runHTTP(c *cli.Context) error {
	return nil
}
