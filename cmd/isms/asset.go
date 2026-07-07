package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func assetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "asset",
		Short: "Manage information assets",
	}

	cmd.AddCommand(assetAddCmd(), assetListCmd(), assetEditCmd(), assetRmCmd())
	return cmd
}

func assetAddCmd() *cobra.Command {
	var (
		name            string
		assetType       string
		owner           string
		description     string
		status          string
		primaryLocation string
		confidentiality int
		integrity       int
		availability    int
		reviewDate      string
		notes           string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an information asset",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || assetType == "" || owner == "" {
				return fmt.Errorf("required: --name, --type, --owner")
			}

			c := requireAPI()
			rd, err := parseEpochPtr(reviewDate)
			if err != nil {
				return err
			}
			asset := &db.Asset{
				Name:            name,
				AssetType:       assetType,
				Owner:           owner,
				Description:     description,
				Status:          status,
				PrimaryLocation: primaryLocation,
				Confidentiality: &confidentiality,
				Integrity:       &integrity,
				Availability:    &availability,
				NextReview:      rd,
				Notes:           notes,
			}
			result, err := c.AddAsset(asset)
			if err != nil {
				return err
			}
			fmt.Printf("Added asset %s: %s\n", result.Identifier, result.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Asset name")
	cmd.Flags().StringVar(&assetType, "type", "", "Asset type: infrastructure, processing_devices, software, financial_info, personal_data, ipr, sales_marketing, processing_facility, products_services, supply_chain, other")
	cmd.Flags().StringVar(&owner, "owner", "", "Asset owner")
	cmd.Flags().StringVar(&description, "desc", "", "Description")
	cmd.Flags().StringVar(&status, "status", "open", "Status: draft, open, archived")
	cmd.Flags().StringVar(&primaryLocation, "location", "", "Primary location: company_office, third_party_dc, on_person, everywhere, other")
	cmd.Flags().IntVar(&confidentiality, "confidentiality", 0, "CIA Confidentiality (0-5)")
	cmd.Flags().IntVar(&integrity, "integrity", 0, "CIA Integrity (0-5)")
	cmd.Flags().IntVar(&availability, "availability", 0, "CIA Availability (0-5)")
	cmd.Flags().StringVar(&reviewDate, "review-date", "", "Next review date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes")
	return cmd
}

func assetListCmd() *cobra.Command {
	var (
		filterType  string
		filterOwner string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List information assets",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			assets, err := c.ListAssets()
			if err != nil {
				return err
			}
			fmt.Printf("%-10s %-30s %-18s %-15s %-6s %3s %3s %3s\n",
				"ID", "NAME", "TYPE", "OWNER", "STATUS", "C", "I", "A")
			fmt.Println(repeat("-", 96))
			count := 0
			for _, a := range assets {
				if filterType != "" && a.AssetType != filterType {
					continue
				}
				if filterOwner != "" && a.Owner != filterOwner {
					continue
				}
				fmt.Printf("%-10s %-30s %-18s %-15s %-6s %3d %3d %3d\n",
					a.Identifier, truncate(a.Name, 30), a.AssetType, truncate(a.Owner, 15),
					truncate(a.Status, 6),
					intVal(a.Confidentiality),
					intVal(a.Integrity),
					intVal(a.Availability))
				count++
			}
			fmt.Printf("\n%d assets\n", count)
			return nil
		},
	}

	cmd.Flags().StringVar(&filterType, "type", "", "Filter by type")
	cmd.Flags().StringVar(&filterOwner, "owner", "", "Filter by owner")
	return cmd
}

// assetEditPayload is the partial-update wire shape for `asset edit` — pointer
// fields with omitempty so an unset flag is omitted rather than sent as a zero
// value the server would write over an existing value (#147). Mirrors the
// server's assetUpdateRequest.
type assetEditPayload struct {
	Name            *string   `json:"name,omitempty"`
	AssetType       *string   `json:"asset_type,omitempty"`
	Owner           *string   `json:"owner,omitempty"`
	Description     *string   `json:"description,omitempty"`
	Status          *string   `json:"status,omitempty"`
	PrimaryLocation *string   `json:"primary_location,omitempty"`
	Confidentiality *int      `json:"confidentiality,omitempty"`
	Integrity       *int      `json:"integrity,omitempty"`
	Availability    *int      `json:"availability,omitempty"`
	NextReview      *db.Epoch `json:"next_review,omitempty"`
	Notes           *string   `json:"notes,omitempty"`
}

func assetEditCmd() *cobra.Command {
	var (
		name            string
		assetType       string
		owner           string
		description     string
		status          string
		primaryLocation string
		confidentiality int
		integrity       int
		availability    int
		reviewDate      string
		notes           string
	)

	cmd := &cobra.Command{
		Use:   "edit <asset-id>",
		Short: "Edit an information asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			// Only fields whose flag was set go on the wire; everything else is
			// omitted so a partial edit never blanks an untouched field (#147).
			update := &assetEditPayload{}
			if cmd.Flags().Changed("name") {
				update.Name = &name
			}
			if cmd.Flags().Changed("type") {
				update.AssetType = &assetType
			}
			if cmd.Flags().Changed("owner") {
				update.Owner = &owner
			}
			if cmd.Flags().Changed("desc") {
				update.Description = &description
			}
			if cmd.Flags().Changed("status") {
				update.Status = &status
			}
			if cmd.Flags().Changed("location") {
				update.PrimaryLocation = &primaryLocation
			}
			if cmd.Flags().Changed("confidentiality") {
				update.Confidentiality = &confidentiality
			}
			if cmd.Flags().Changed("integrity") {
				update.Integrity = &integrity
			}
			if cmd.Flags().Changed("availability") {
				update.Availability = &availability
			}
			if cmd.Flags().Changed("review-date") {
				rd, err := parseEpochPtr(reviewDate)
				if err != nil {
					return err
				}
				update.NextReview = rd
			}
			if cmd.Flags().Changed("notes") {
				update.Notes = &notes
			}
			if _, err := c.UpdateAsset(id, update); err != nil {
				return err
			}
			fmt.Printf("Updated %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Asset name")
	cmd.Flags().StringVar(&assetType, "type", "", "Asset type")
	cmd.Flags().StringVar(&owner, "owner", "", "Asset owner")
	cmd.Flags().StringVar(&description, "desc", "", "Description")
	cmd.Flags().StringVar(&status, "status", "", "Status: draft, open, archived")
	cmd.Flags().StringVar(&primaryLocation, "location", "", "Primary location")
	cmd.Flags().IntVar(&confidentiality, "confidentiality", 0, "CIA Confidentiality (0-5)")
	cmd.Flags().IntVar(&integrity, "integrity", 0, "CIA Integrity (0-5)")
	cmd.Flags().IntVar(&availability, "availability", 0, "CIA Availability (0-5)")
	cmd.Flags().StringVar(&reviewDate, "review-date", "", "Next review date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes")
	return cmd
}

func assetRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <asset-id>",
		Short: "Remove an information asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			if err := c.DeleteAsset(id); err != nil {
				return err
			}
			fmt.Printf("Removed %s\n", id)
			return nil
		},
	}
}
