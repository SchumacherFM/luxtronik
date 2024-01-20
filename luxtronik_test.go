package luxtronik

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Client(t *testing.T) {
	heatPumpIP := os.Getenv("HEATPUMP_IP")
	if heatPumpIP == "" {
		heatPumpIP = "192.168.0.121" + ":" + DefaultPort
	}

	runTest := func(newMap func() DataTypeMap, readFromNet func(*Client, DataTypeMap) error) func(t *testing.T) {
		return func(t *testing.T) {
			c := MustNewClient(heatPumpIP, Options{
				SafeMode: true,
			})

			require.NoError(t, c.Connect())
			defer func() {
				assert.NoError(t, c.Close())
			}()

			pm := newMap()
			require.NoError(t, readFromNet(c, pm))

			tw := tabwriter.NewWriter(os.Stdout, 12, 1, 1, ' ', 0)
			printFn := func(w io.Writer) func(i int, p *Base) {
				return func(i int, p *Base) {
					if p.rawValue == 0 {
						return
					}

					fmt.Fprintf(
						w,
						"Number: %d\tName: %s\tType: %s\tValue: %v\tUnit: %s\n",
						i,
						p.luxtronikName,
						p.class,
						checkStringer(p.FromHeatPump()),
						p.unit,
					)
				}
			}
			pm.IterateSorted(printFn(tw))
			require.NoError(t, tw.Flush())
		}
	}

	t.Run("Parameter", runTest(NewParameterMap, func(c *Client, pm DataTypeMap) error {
		return c.readParameters(pm)
	}))
	t.Run("Visibilities", runTest(NewVisibilitiesMap, func(c *Client, pm DataTypeMap) error {
		return c.readVisibilities(pm)
	}))
	t.Run("Calculations", runTest(NewCalculationsMap, func(c *Client, pm DataTypeMap) error {
		return c.readCalculations(pm)
	}))
}

func TestIntegration_Refreshed_Calculations(t *testing.T) {
	c := MustNewClient("192.168.0.121:"+DefaultPort, Options{
		SafeMode: true,
	})

	require.NoError(t, c.Connect())
	defer func() {
		assert.NoError(t, c.Close())
	}()

	pm := NewCalculationsMap()

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt)
	tkr := time.NewTicker(3 * time.Second)

	for {

		require.NoError(t, c.readCalculations(pm))

		tw := tabwriter.NewWriter(os.Stdout, 12, 1, 1, ' ', 0)

		pm.IterateSorted(func(i int, p *Base) {
			if p.rawValue == 0 || !p.HasChanges() {
				return
			}

			fmt.Fprintf(
				tw,
				"Number: %d\tName: %s\tType: %s\tValue: %v\tUnit: %s\n",
				i,
				p.luxtronikName,
				p.class,
				checkStringer(p.FromHeatPump()),
				p.unit,
			)
		})

		require.NoError(t, tw.Flush())
		select {
		case <-sigChan:
			return
		case tm := <-tkr.C:
			println(tm.Format(time.DateTime), strings.Repeat("=", 200))
			continue
		}
	}
}

func checkStringer(v any) any {
	if s, ok := v.(fmt.Stringer); ok {
		return s.String()
	}
	switch tv := v.(type) {
	case float32:
		return fmt.Sprintf("%.3f", tv)
	default:
		return v
	}
}
