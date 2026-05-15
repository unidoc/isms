package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current user and verify API connection",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			info, err := c.WhoAmI()
			if err != nil {
				return fmt.Errorf("API connection failed: %w", err)
			}
			fmt.Printf("Server:       %s\n", c.BaseURL())
			fmt.Printf("Email:        %s\n", info.Email)
			fmt.Printf("Name:         %s\n", info.Name)
			if info.OrganizationSlug != "" {
				fmt.Printf("Organization: %s (%s)\n", info.OrganizationName, info.OrganizationSlug)
				fmt.Printf("Org UUID:     %s\n", info.OrganizationUUID)
				fmt.Printf("Role:         %s\n", info.Role)
			} else {
				// No org resolved — list available orgs
				orgs, err := c.ListMyOrgs()
				if err == nil && len(orgs) > 0 {
					fmt.Printf("\nOrganizations:\n")
					for _, o := range orgs {
						fmt.Printf("  %s  %s (%s)  [%s]\n", o.UUID, o.Name, o.Slug, o.Role)
					}
					fmt.Printf("\nSet ISMS_ORGANIZATION=<uuid> in your env file to select one.\n")
				} else {
					fmt.Printf("\nNo organizations found.\n")
				}
			}
			return nil
		},
	}
}
