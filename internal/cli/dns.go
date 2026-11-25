package cli

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/laurenschristian/eerox/internal/eero"
)

// maxCustomDNS is how many IPv4 resolvers eero accepts in custom mode.
const maxCustomDNS = 2

func dnsCmd() *cobra.Command {
	var set []string
	var auto, yes bool
	c := &cobra.Command{
		Use:   "dns",
		Short: "Show or set the network DNS resolvers (--set to pin, --auto for ISP default)",
		Long: "Show or set the network's IPv4 DNS resolvers.\n\n" +
			"Changing DNS reboots every eero in the mesh a few minutes after the change is accepted,\n" +
			"so Wi-Fi and internet drop briefly for everyone. eerox skips the write when the\n" +
			"setting already matches; avoid repeated changes, since each one queues another reboot.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := ctx()
			defer cancel()
			id, err := netID(ctx)
			if err != nil {
				return err
			}
			if auto && len(set) > 0 {
				return fmt.Errorf("use either --set or --auto, not both")
			}
			if auto || len(set) > 0 {
				mode, ips := "automatic", []string(nil)
				if !auto {
					mode = "custom"
					if ips, err = parseResolvers(set); err != nil {
						return err
					}
				}
				n, err := client.Network(ctx, id)
				if err != nil {
					return err
				}
				if dnsMatches(n, mode, ips) {
					fmt.Println("dns already set as requested; nothing changed (a write would reboot the mesh)")
				} else {
					if !yes {
						return fmt.Errorf("changing DNS reboots every eero a few minutes later (Wi-Fi drops for all clients): re-run with --yes")
					}
					if err := client.SetDNS(ctx, id, mode, ips); err != nil {
						return err
					}
					n, err = client.Network(ctx, id)
					if err != nil {
						return fmt.Errorf("dns write sent, but reading it back failed: %w", err)
					}
					if !dnsMatches(n, mode, ips) {
						return fmt.Errorf("eero accepted the dns write but did not apply it (now %s %s)", n.DNS.Mode, strings.Join(currentIPs(n), ", "))
					}
					fmt.Println("dns updated; the eeros will reboot to apply it within a few minutes")
				}
			}
			n, err := client.Network(ctx, id)
			if err != nil {
				return err
			}
			if flagJSON {
				return emit(n.DNS)
			}
			ips := currentIPs(n)
			table([][]string{
				{"mode", n.DNS.Mode},
				{"resolvers", strings.Join(ips, ", ")},
				{"caching", fmt.Sprint(n.DNS.Caching)},
			})
			if len(ips) > 1 && n.DNS.Mode == "custom" {
				fmt.Println("\nnote: clients may query any listed resolver, so a single private resolver enforces filtering.")
			}
			return nil
		},
	}
	c.Flags().StringSliceVar(&set, "set", nil, "custom IPv4 resolver IPs (at most 2), e.g. --set 10.0.4.70")
	c.Flags().BoolVar(&auto, "auto", false, "revert to the ISP/eero default resolvers")
	c.Flags().BoolVar(&yes, "yes", false, "confirm the change (it reboots the mesh)")
	return c
}

// parseResolvers validates --set values: IPv4 only, no duplicates, at most maxCustomDNS.
func parseResolvers(in []string) ([]string, error) {
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		a, err := netip.ParseAddr(s)
		if err != nil || !a.Is4() {
			return nil, fmt.Errorf("%q is not an IPv4 address (eerox sets IPv4 resolvers only)", s)
		}
		if slices.Contains(out, a.String()) {
			return nil, fmt.Errorf("resolver %s listed twice", s)
		}
		out = append(out, a.String())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--set needs at least one resolver")
	}
	if len(out) > maxCustomDNS {
		return nil, fmt.Errorf("eero accepts at most %d custom resolvers, got %d", maxCustomDNS, len(out))
	}
	return out, nil
}

// currentIPs returns the resolvers the network is configured with.
func currentIPs(n *eero.Network) []string {
	if n.DNS.Custom != nil && len(n.DNS.Custom.IPs) > 0 {
		return n.DNS.Custom.IPs
	}
	return n.DNS.Parent.IPs
}

// dnsMatches reports whether the network already has the requested DNS setting.
func dnsMatches(n *eero.Network, mode string, ips []string) bool {
	if n.DNS.Mode != mode {
		return false
	}
	if mode != "custom" {
		return true
	}
	return n.DNS.Custom != nil && slices.Equal(n.DNS.Custom.IPs, ips)
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config, session and reachability",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := ctx()
			defer cancel()
			ok := func(b bool) string {
				if b {
					return "ok"
				}
				return "FAIL"
			}
			hasToken := cfg.Token != ""
			fmt.Printf("%-5s config       %s\n", ok(true), cfgPath())
			fmt.Printf("%-5s session      %s\n", ok(hasToken), sessionWhere())
			if !hasToken {
				fmt.Println("\nrun `eerox login <email-or-phone>`")
				return nil
			}
			a, err := client.Account(ctx)
			fmt.Printf("%-5s api          %s\n", ok(err == nil), accountLine(a, err))
			if err != nil {
				return nil
			}
			id, nerr := netID(ctx)
			fmt.Printf("%-5s network      %s\n", ok(nerr == nil), id)
			if nerr == nil {
				if n, e := client.Network(ctx, id); e == nil {
					fmt.Printf("%-5s internet     %s\n", ok(n.Health.Internet.Status == "connected"), n.Health.Internet.Status)
				}
			}
			return nil
		},
	}
}
