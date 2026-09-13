package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/client"
	"isms.sh/internal/isms/db"
	riskpkg "isms.sh/internal/isms/risk"
)

// riskCategoryFlagHelp derives the --category help text from the default
// category list so it can never drift, and notes that an org may define its own.
func riskCategoryFlagHelp() string {
	return fmt.Sprintf("Category (defaults: %s; your organization may define its own)",
		strings.Join(db.RiskCategories, ", "))
}

func riskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "risk",
		Short: "Manage risk register",
	}

	cmd.AddCommand(riskAddCmd(), riskListCmd(), riskAssessCmd(), riskTreatCmd(), riskMatrixCmd())
	return cmd
}

func riskAddCmd() *cobra.Command {
	var (
		title                         string
		description                   string
		riskType                      string
		origin                        string
		category                      string
		assets                        []string
		currentLikelihood             int
		currentImpact                 int
		confidentialityImpact         int
		integrityImpact               int
		availabilityImpact            int
		inherentLikelihood            int
		inherentImpact                int
		inherentConfidentialityImpact int
		inherentIntegrityImpact       int
		inherentAvailabilityImpact    int
		targetLikelihood              int
		targetImpact                  int
		treatment                     string
		treatmentPlan                 string
		linkedDocs                    []string
		owner                         string
		status                        string
		reviewDate                    string
		notes                         string
		fields                        []string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a risk to the register",
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" || owner == "" || currentLikelihood == 0 || currentImpact == 0 {
				return fmt.Errorf("required: --title, --owner, --likelihood, --impact")
			}
			if currentLikelihood < 1 || currentLikelihood > 5 || currentImpact < 1 || currentImpact > 5 {
				return fmt.Errorf("--likelihood and --impact must be 1-5")
			}

			c := requireAPI()
			rd, err := parseEpochPtr(reviewDate)
			if err != nil {
				return err
			}
			customFields, err := parseRiskCustomFieldFlags(c, fields)
			if err != nil {
				return err
			}
			r := &db.Risk{
				Title:                         title,
				Description:                   description,
				RiskType:                      riskType,
				Origin:                        origin,
				Category:                      category,
				CustomFields:                  customFields,
				CurrentLikelihood:             &currentLikelihood,
				CurrentImpact:                 &currentImpact,
				ConfidentialityImpact:         &confidentialityImpact,
				IntegrityImpact:               &integrityImpact,
				AvailabilityImpact:            &availabilityImpact,
				InherentLikelihood:            &inherentLikelihood,
				InherentImpact:                &inherentImpact,
				InherentConfidentialityImpact: &inherentConfidentialityImpact,
				InherentIntegrityImpact:       &inherentIntegrityImpact,
				InherentAvailabilityImpact:    &inherentAvailabilityImpact,
				TargetLikelihood:              &targetLikelihood,
				TargetImpact:                  &targetImpact,
				Treatment:                     treatment,
				TreatmentPlan:                 treatmentPlan,
				Owner:                         owner,
				Status:                        status,
				NextReview:                    rd,
				Notes:                         notes,
			}
			result, err := c.AddRisk(r, buildRefs(
				refSpec{"asset", assets},
				refSpec{"document", linkedDocs},
			))
			if err != nil {
				return err
			}
			fmt.Printf("Added %s: %s — Score: %d (%s)\n", result.Identifier, result.Title, intVal(result.CurrentScore), result.CurrentLevel)
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Risk title")
	cmd.Flags().StringVar(&description, "desc", "", "Description of what could happen")
	cmd.Flags().StringVar(&riskType, "risk-type", "threat", "Risk type: threat, opportunity")
	cmd.Flags().StringVar(&origin, "origin", "internal", "Origin: internal, external, 'internal and external'")
	cmd.Flags().StringVar(&category, "category", "", riskCategoryFlagHelp())
	cmd.Flags().StringSliceVar(&assets, "assets", nil, "Affected asset IDs (comma-separated)")
	cmd.Flags().IntVar(&currentLikelihood, "likelihood", 0, "Current likelihood (1-5)")
	cmd.Flags().IntVar(&currentImpact, "impact", 0, "Current impact (1-5)")
	cmd.Flags().IntVar(&confidentialityImpact, "confidentiality", 0, "CIA Confidentiality impact (0-5)")
	cmd.Flags().IntVar(&integrityImpact, "integrity", 0, "CIA Integrity impact (0-5)")
	cmd.Flags().IntVar(&availabilityImpact, "availability", 0, "CIA Availability impact (0-5)")
	cmd.Flags().IntVar(&inherentLikelihood, "inherent-likelihood", 0, "Inherent likelihood (1-5)")
	cmd.Flags().IntVar(&inherentImpact, "inherent-impact", 0, "Inherent impact (1-5)")
	cmd.Flags().IntVar(&inherentConfidentialityImpact, "inherent-confidentiality", 0, "Inherent CIA Confidentiality impact (0-5)")
	cmd.Flags().IntVar(&inherentIntegrityImpact, "inherent-integrity", 0, "Inherent CIA Integrity impact (0-5)")
	cmd.Flags().IntVar(&inherentAvailabilityImpact, "inherent-availability", 0, "Inherent CIA Availability impact (0-5)")
	cmd.Flags().IntVar(&targetLikelihood, "target-likelihood", 0, "Target likelihood (1-5)")
	cmd.Flags().IntVar(&targetImpact, "target-impact", 0, "Target impact (1-5)")
	cmd.Flags().StringVar(&treatment, "treatment", "mitigate", "Treatment: accept, mitigate, transfer, avoid")
	cmd.Flags().StringVar(&treatmentPlan, "plan", "", "Treatment plan")
	cmd.Flags().StringSliceVar(&linkedDocs, "documents", nil, "Linked document IDs (comma-separated)")
	cmd.Flags().StringVar(&owner, "owner", "", "Risk owner")
	cmd.Flags().StringVar(&status, "status", "open", "Status: draft, open, closed")
	cmd.Flags().StringVar(&reviewDate, "review-date", "", "Next review date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "Custom field value as key=value (repeatable)")
	return cmd
}

// parseRiskCustomFieldFlags turns repeated --field key=value flags into the
// payload the API expects, coercing each value per its definition's Type the
// same way the web form does (number -> float64, others -> string). It fails
// fast client-side, listing missing required fields by key and label, rather
// than forwarding the server's generic 400 — required custom fields ship in
// v1, so the CLI needs its own way to supply them.
func parseRiskCustomFieldFlags(c *client.Client, fields []string) (map[string]any, error) {
	values := make(map[string]string, len(fields))
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return nil, fmt.Errorf("--field must be key=value, got %q", f)
		}
		values[k] = v
	}

	defs, err := c.GetRiskCustomFields()
	if err != nil {
		// If we can't resolve definitions, fall back to sending strings as-is
		// and let the server validate.
		out := make(map[string]any, len(values))
		for k, v := range values {
			out[k] = v
		}
		return out, nil
	}

	out := make(map[string]any, len(values))
	for k, v := range values {
		def, ok := customFieldDefByKeyCLI(defs, k)
		if !ok {
			out[k] = v
			continue
		}
		if def.Type == "number" {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("--field %s: %q is not a number", k, v)
			}
			out[k] = n
		} else {
			out[k] = v
		}
	}

	var missing []string
	for _, def := range defs {
		if !def.Required {
			continue
		}
		v, ok := values[def.Key]
		if !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, fmt.Sprintf("%s (%s)", def.Key, def.Label))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required custom field(s): %s (use --field key=value)", strings.Join(missing, ", "))
	}

	return out, nil
}

func customFieldDefByKeyCLI(defs []db.CustomFieldDef, key string) (db.CustomFieldDef, bool) {
	for _, d := range defs {
		if d.Key == key {
			return d, true
		}
	}
	return db.CustomFieldDef{}, false
}

func riskListCmd() *cobra.Command {
	var (
		filterLevel  string
		filterOwner  string
		filterStatus string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List risks",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			risks, err := c.ListRisks()
			if err != nil {
				return err
			}
			fmt.Printf("%-10s %-30s %-8s %-8s %-8s %3s %3s %5s %-8s %-10s %-12s\n",
				"ID", "TITLE", "TYPE", "ORIGIN", "CATEGORY", "L", "I", "SCORE", "LEVEL", "STATUS", "OWNER")
			fmt.Println(repeat("-", 120))
			count := 0
			for _, r := range risks {
				if filterLevel != "" && r.CurrentLevel != filterLevel {
					continue
				}
				if filterOwner != "" && r.Owner != filterOwner {
					continue
				}
				if filterStatus != "" && r.Status != filterStatus {
					continue
				}
				fmt.Printf("%-10s %-30s %-8s %-8s %-8s %3d %3d %5d %-8s %-10s %-12s\n",
					r.Identifier, truncate(r.Title, 30),
					truncate(r.RiskType, 8), truncate(r.Origin, 8), truncate(r.Category, 8),
					intVal(r.CurrentLikelihood), intVal(r.CurrentImpact), intVal(r.CurrentScore), r.CurrentLevel, r.Status, truncate(r.Owner, 12))
				count++
			}
			fmt.Printf("\n%d risks\n", count)
			return nil
		},
	}

	cmd.Flags().StringVar(&filterLevel, "level", "", "Filter by level: Low, Medium, High, Critical")
	cmd.Flags().StringVar(&filterOwner, "owner", "", "Filter by owner")
	cmd.Flags().StringVar(&filterStatus, "status", "", "Filter by status")
	return cmd
}

func riskAssessCmd() *cobra.Command {
	var (
		currentLikelihood int
		currentImpact     int
	)

	cmd := &cobra.Command{
		Use:   "assess <risk-id>",
		Short: "Reassess a risk (update likelihood/impact)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			update := &db.Risk{}
			if cmd.Flags().Changed("likelihood") {
				update.CurrentLikelihood = &currentLikelihood
			}
			if cmd.Flags().Changed("impact") {
				update.CurrentImpact = &currentImpact
			}
			result, err := c.UpdateRisk(id, update)
			if err != nil {
				return err
			}
			fmt.Printf("%s reassessed — Score: %d (%s)\n", id, intVal(result.CurrentScore), result.CurrentLevel)
			return nil
		},
	}

	cmd.Flags().IntVar(&currentLikelihood, "likelihood", 0, "New current likelihood (1-5)")
	cmd.Flags().IntVar(&currentImpact, "impact", 0, "New current impact (1-5)")
	return cmd
}

func riskTreatCmd() *cobra.Command {
	var (
		decision string
	)

	cmd := &cobra.Command{
		Use:   "treat <risk-id>",
		Short: "Set risk treatment decision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c := requireAPI()
			update := &db.Risk{
				Treatment: decision,
			}
			if decision == "accept" {
				update.Status = "accepted"
			} else {
				update.Status = "treating"
			}
			if _, err := c.UpdateRisk(id, update); err != nil {
				return err
			}
			fmt.Printf("%s — treatment: %s\n", id, decision)
			return nil
		},
	}

	cmd.Flags().StringVar(&decision, "decision", "", "Treatment: accept, mitigate, transfer, avoid")
	return cmd
}

func riskMatrixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "matrix",
		Short: "Display the 5x5 risk matrix",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(riskpkg.PrintMatrix())
			return nil
		},
	}
}
