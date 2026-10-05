package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/db"
)

func legalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "legal",
		Short: "Legal register — applicable legislation and risk assessment",
	}

	cmd.AddCommand(
		legalAddCmd(),
		legalListCmd(),
		legalShowCmd(),
		legalUpdateCmd(),
		legalRmCmd(),
	)
	return cmd
}

func legalAddCmd() *cobra.Command {
	var title, description, jurisdiction, category, reference, url string
	var owner, notes, treatment, treatmentPlan string
	var linkedDocs []string
	var currentLikelihood, currentImpact, completion int

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a legal requirement",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()

			lr := &db.LegalRequirement{
				Title:             title,
				Description:       description,
				Jurisdiction:      jurisdiction,
				Category:          category,
				Reference:         reference,
				URL:               url,
				Owner:             owner,
				Notes:             notes,
				CurrentLikelihood: &currentLikelihood,
				CurrentImpact:     &currentImpact,
				Treatment:         treatment,
				TreatmentPlan:     treatmentPlan,
				Completion:        completion,
			}

			result, err := c.CreateLegal(lr, buildRefs(
				refSpec{"document", linkedDocs},
			))
			if err != nil {
				return err
			}
			fmt.Printf("Legal requirement #%d created: %s\n", result.ID, result.Title)
			fmt.Printf("  Jurisdiction: %s\n", result.Jurisdiction)
			fmt.Printf("  Category:     %s\n", result.Category)
			if result.Owner != "" {
				fmt.Printf("  Owner:        %s\n", result.Owner)
			}
			if intVal(result.CurrentScore) > 0 {
				fmt.Printf("  Risk Score:   %d (%s)\n", intVal(result.CurrentScore), result.CurrentLevel)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Legislation title (e.g. GDPR, NIS2)")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&jurisdiction, "jurisdiction", "EU", "Jurisdiction: ISO 3166-1 alpha-2 country code (IS, US, DE) or a region (Global, EU, EEA, APAC)")
	cmd.Flags().StringVar(&category, "category", "privacy", "Category: privacy, security, sector, contractual, other")
	cmd.Flags().StringVar(&reference, "reference", "", "Article/section reference")
	cmd.Flags().StringVar(&url, "url", "", "Link to legislation text")
	cmd.Flags().StringVar(&owner, "owner", "", "Responsible person email")
	cmd.Flags().StringSliceVar(&linkedDocs, "documents", nil, "Linked document IDs (comma-separated)")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes")
	cmd.Flags().IntVar(&currentLikelihood, "likelihood", 0, "Likelihood (1-5)")
	cmd.Flags().IntVar(&currentImpact, "impact", 0, "Impact (1-5)")
	cmd.Flags().StringVar(&treatment, "treatment", "", "Treatment: mitigate, accept, transfer, avoid")
	cmd.Flags().StringVar(&treatmentPlan, "treatment-plan", "", "Treatment plan (markdown supported)")
	cmd.Flags().IntVar(&completion, "completion", 0, "Completion percentage (0-100)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func legalListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List legal requirements",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			items, err := c.ListLegal("")
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No legal requirements found.")
				return nil
			}
			fmt.Printf("  %-6s %-30s %-12s %-12s %5s %-8s %-24s\n",
				"ID", "TITLE", "JURISDICTION", "CATEGORY", "RISK", "TREAT", "OWNER")
			fmt.Printf("  %s\n", strings.Repeat("-", 106))
			for _, lr := range items {
				riskStr := "-"
				if intVal(lr.CurrentScore) > 0 {
					riskStr = fmt.Sprintf("%d", intVal(lr.CurrentScore))
				}
				fmt.Printf("  %-6d %-30s %-12s %-12s %5s %-8s %-24s\n",
					lr.ID,
					truncate(lr.Title, 30),
					truncate(lr.Jurisdiction, 12),
					truncate(lr.Category, 12),
					riskStr,
					truncate(lr.Treatment, 8),
					truncate(lr.Owner, 24),
				)
			}
			fmt.Printf("\n%d legal requirement(s)\n", len(items))
			return nil
		},
	}
	return cmd
}

func legalShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show legal requirement details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid legal requirement ID: %s", args[0])
			}

			c := requireAPI()
			lr, err := c.GetLegal(id)
			if err != nil {
				return err
			}

			fmt.Printf("Legal Requirement #%d\n", lr.ID)
			fmt.Printf("  Title:        %s\n", lr.Title)
			if lr.Description != "" {
				fmt.Printf("  Description:  %s\n", lr.Description)
			}
			fmt.Printf("  Jurisdiction: %s\n", lr.Jurisdiction)
			fmt.Printf("  Category:     %s\n", lr.Category)
			if lr.Reference != "" {
				fmt.Printf("  Reference:    %s\n", lr.Reference)
			}
			if lr.URL != "" {
				fmt.Printf("  URL:          %s\n", lr.URL)
			}
			if lr.Owner != "" {
				fmt.Printf("  Owner:        %s\n", lr.Owner)
			}
			if lr.LastReview != nil && !lr.LastReview.IsZero() {
				fmt.Printf("  Last Review:  %s\n", lr.LastReview.Format("2006-01-02"))
			}
			if lr.NextReview != nil && !lr.NextReview.IsZero() {
				fmt.Printf("  Next Review:  %s\n", lr.NextReview.Format("2006-01-02"))
			}
			// Risk assessment
			if intVal(lr.CurrentLikelihood) > 0 || intVal(lr.CurrentImpact) > 0 {
				fmt.Printf("  --- Risk Assessment ---\n")
				fmt.Printf("  Likelihood:   %d\n", intVal(lr.CurrentLikelihood))
				fmt.Printf("  Impact:       %d\n", intVal(lr.CurrentImpact))
				fmt.Printf("  Risk Score:   %d (%s)\n", intVal(lr.CurrentScore), lr.CurrentLevel)
			}
			if lr.Treatment != "" {
				fmt.Printf("  Treatment:    %s\n", lr.Treatment)
			}
			if intVal(lr.TargetLikelihood) > 0 || intVal(lr.TargetImpact) > 0 {
				fmt.Printf("  Target L/I:   %d / %d\n", intVal(lr.TargetLikelihood), intVal(lr.TargetImpact))
			}
			if lr.Completion > 0 {
				fmt.Printf("  Completion:   %d%%\n", lr.Completion)
			}
			if lr.Notes != "" {
				fmt.Printf("  Notes:        %s\n", lr.Notes)
			}
			fmt.Printf("  Created:      %s\n", lr.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Printf("  Updated:      %s\n", lr.UpdatedAt.Format("2006-01-02 15:04"))
			return nil
		},
	}
}

// legalUpdatePayload is the partial-update wire shape for `legal update`:
// pointer fields with omitempty, filled only for flags the user set. Sending
// a whole db.LegalRequirement put every unset field on the wire as "" or 0,
// which the server stored as a real value and the row's CHECK constraints
// then rejected (#385). Mirrors the server's legalUpdateRequest.
//
// The two dates are double pointers so a date flag given as "" is told apart
// from an absent one: a nil outer pointer omits the key, a pointer to nil is
// sent as null, which clears last_review and recalculates next_review.
type legalUpdatePayload struct {
	Title             *string    `json:"title,omitempty"`
	Description       *string    `json:"description,omitempty"`
	Jurisdiction      *string    `json:"jurisdiction,omitempty"`
	Category          *string    `json:"category,omitempty"`
	Reference         *string    `json:"reference,omitempty"`
	URL               *string    `json:"url,omitempty"`
	Owner             *string    `json:"owner,omitempty"`
	LastReview        **db.Epoch `json:"last_review,omitempty"`
	NextReview        **db.Epoch `json:"next_review,omitempty"`
	Notes             *string    `json:"notes,omitempty"`
	CurrentLikelihood *int       `json:"current_likelihood,omitempty"`
	CurrentImpact     *int       `json:"current_impact,omitempty"`
	Treatment         *string    `json:"treatment,omitempty"`
	TreatmentPlan     *string    `json:"treatment_plan,omitempty"`
	Completion        *int       `json:"completion,omitempty"`
	TargetLikelihood  *int       `json:"target_likelihood,omitempty"`
	TargetImpact      *int       `json:"target_impact,omitempty"`
}

func legalUpdateCmd() *cobra.Command {
	var title, description, jurisdiction, category, reference, url string
	var owner, notes, lastReview, nextReview, treatment, treatmentPlan string
	var currentLikelihood, currentImpact, completion int
	var targetLikelihood, targetImpact int

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a legal requirement",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid legal requirement ID: %s", args[0])
			}

			c := requireAPI()
			// Only fields whose flag was set go on the wire, so a partial
			// update never overwrites an untouched field (#385).
			update := &legalUpdatePayload{}
			f := cmd.Flags()
			if f.Changed("title") {
				update.Title = &title
			}
			if f.Changed("description") {
				update.Description = &description
			}
			if f.Changed("jurisdiction") {
				update.Jurisdiction = &jurisdiction
			}
			if f.Changed("category") {
				update.Category = &category
			}
			if f.Changed("reference") {
				update.Reference = &reference
			}
			if f.Changed("url") {
				update.URL = &url
			}
			if f.Changed("owner") {
				update.Owner = &owner
			}
			if f.Changed("last-review") {
				e, err := parseEpochPtr(lastReview)
				if err != nil {
					return err
				}
				update.LastReview = &e
			}
			if f.Changed("next-review") {
				e, err := parseEpochPtr(nextReview)
				if err != nil {
					return err
				}
				update.NextReview = &e
			}
			if f.Changed("notes") {
				update.Notes = &notes
			}
			if f.Changed("likelihood") {
				update.CurrentLikelihood = &currentLikelihood
			}
			if f.Changed("impact") {
				update.CurrentImpact = &currentImpact
			}
			if f.Changed("treatment") {
				update.Treatment = &treatment
			}
			if f.Changed("treatment-plan") {
				update.TreatmentPlan = &treatmentPlan
			}
			if f.Changed("completion") {
				update.Completion = &completion
			}
			if f.Changed("target-likelihood") {
				update.TargetLikelihood = &targetLikelihood
			}
			if f.Changed("target-impact") {
				update.TargetImpact = &targetImpact
			}
			if err := c.UpdateLegal(id, update); err != nil {
				return err
			}
			fmt.Printf("Legal requirement #%d updated.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Legislation title")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&jurisdiction, "jurisdiction", "", "Jurisdiction: ISO 3166-1 alpha-2 country code (IS, US, DE) or a region (Global, EU, EEA, APAC)")
	cmd.Flags().StringVar(&category, "category", "", "Category")
	cmd.Flags().StringVar(&reference, "reference", "", "Article/section reference")
	cmd.Flags().StringVar(&url, "url", "", "URL")
	cmd.Flags().StringVar(&owner, "owner", "", "Owner email")
	cmd.Flags().StringVar(&lastReview, "last-review", "", `Last review date (YYYY-MM-DD); "" clears it`)
	cmd.Flags().StringVar(&nextReview, "next-review", "", `Next review date (YYYY-MM-DD); "" resets it to the date the review cycle gives`)
	cmd.Flags().StringVar(&notes, "notes", "", "Notes")
	cmd.Flags().IntVar(&currentLikelihood, "likelihood", 0, "Likelihood (1-5)")
	cmd.Flags().IntVar(&currentImpact, "impact", 0, "Impact (1-5)")
	cmd.Flags().StringVar(&treatment, "treatment", "", "Treatment: mitigate, accept, transfer, avoid")
	cmd.Flags().StringVar(&treatmentPlan, "treatment-plan", "", "Treatment plan (markdown supported)")
	cmd.Flags().IntVar(&completion, "completion", 0, "Completion percentage (0-100)")
	cmd.Flags().IntVar(&targetLikelihood, "target-likelihood", 0, "Target likelihood (1-5)")
	cmd.Flags().IntVar(&targetImpact, "target-impact", 0, "Target impact (1-5)")
	return cmd
}

func legalRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a legal requirement",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid legal requirement ID: %s", args[0])
			}

			c := requireAPI()
			if err := c.DeleteLegal(id); err != nil {
				return err
			}
			fmt.Printf("Legal requirement #%d deleted.\n", id)
			return nil
		},
	}
}
