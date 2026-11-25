package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/laurenschristian/eerox/internal/eero"
)

// toggleFlags lists the `set` flags in display order.
var toggleFlags = []string{"band-steering", "ipv6", "sqm", "thread", "upnp", "wpa3"}

// toggleTarget returns the NetworkToggles field a `set` flag controls.
func toggleTarget(t *eero.NetworkToggles, flag string) **bool {
	switch flag {
	case "wpa3":
		return &t.WPA3
	case "sqm":
		return &t.SQM
	case "band-steering":
		return &t.BandSteering
	case "upnp":
		return &t.UPnP
	case "ipv6":
		return &t.IPv6
	case "thread":
		return &t.Thread
	}
	return nil
}

func setCmd() *cobra.Command {
	vals := map[string]*bool{}
	var yes bool
	c := &cobra.Command{
		Use:   "set",
		Short: "Toggle network features (--wpa3, --sqm, --band-steering, --upnp, --ipv6, --thread)",
		Long: "Turn network features on or off, e.g. `eerox set --wpa3 on --sqm on --yes`.\n" +
			"With no flags it prints the current state. WPA3 can disconnect legacy 2.4GHz gear;\n" +
			"SQM (smart queue management) trades a little peak throughput for lower latency under load.\n\n" +
			"Settings changes may reboot every eero in the mesh a few minutes later (confirmed for DNS\n" +
			"on the same endpoint). Only settings that differ are written, and each is read back.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := ctx()
			defer cancel()
			id, err := netID(ctx)
			if err != nil {
				return err
			}
			var want eero.NetworkToggles
			pending := false
			for _, f := range toggleFlags {
				if cmd.Flags().Changed(f) {
					v := *vals[f]
					*toggleTarget(&want, f) = &v
					pending = true
				}
			}
			if pending {
				if !yes {
					return fmt.Errorf("changing network settings affects every client and may reboot the eeros: re-run with --yes")
				}
				changed, err := client.SetNetworkToggles(ctx, id, want)
				if err != nil {
					if len(changed) > 0 {
						return fmt.Errorf("after writing %s: %w", strings.Join(changed, ", "), err)
					}
					return err
				}
				if len(changed) == 0 {
					fmt.Println("already set as requested; nothing changed")
				} else {
					fmt.Printf("updated %d setting(s): %s (the eeros may reboot within a few minutes)\n", len(changed), strings.Join(changed, ", "))
				}
			}
			n, err := client.Network(ctx, id)
			if err != nil {
				return err
			}
			state := map[string]bool{"wpa3": n.Wpa3, "sqm": n.SQM, "band_steering": n.BandSteering, "upnp": n.Upnp, "ipv6_upstream": n.IPv6Upstream}
			if th, err := client.Thread(ctx, id); err == nil {
				state["thread"] = th.Enabled
			}
			if flagJSON {
				return emit(state)
			}
			keys := make([]string, 0, len(state))
			for k := range state {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			rows := make([][]string, 0, len(keys))
			for _, k := range keys {
				rows = append(rows, []string{k, onOff(state[k])})
			}
			table(rows)
			return nil
		},
	}
	for _, f := range toggleFlags {
		b := new(bool)
		vals[f] = b
		c.Flags().Var(&onOffValue{b}, f, "on|off")
	}
	c.Flags().BoolVar(&yes, "yes", false, "confirm the change (it may reboot the mesh)")
	return c
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// onOffValue is a pflag.Value accepting on/off/true/false/1/0.
type onOffValue struct{ p *bool }

func (o *onOffValue) String() string { return onOff(o.p != nil && *o.p) }
func (o *onOffValue) Type() string   { return "on|off" }
func (o *onOffValue) Set(s string) error {
	switch strings.ToLower(s) {
	case "on", "true", "1", "yes", "enable", "enabled":
		*o.p = true
	case "off", "false", "0", "no", "disable", "disabled":
		*o.p = false
	default:
		return fmt.Errorf("want on or off, got %q", s)
	}
	return nil
}
