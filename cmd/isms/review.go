package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/client"
)

func reviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Document review workflow",
	}

	cmd.AddCommand(reviewSendCmd(), reviewListCmd(), reviewShowCmd(), reviewCloseCmd(), reviewApproveCmd(), reviewAssignCmd())
	return cmd
}

func reviewSendCmd() *cobra.Command {
	var reviewers []string

	cmd := &cobra.Command{
		Use:   "send <document-id>",
		Short: "Send a document for review",
		Long: `Creates a review request. Optionally assign reviewers with --to.
If no reviewers specified, the review is created for self-check.
Use 'isms review forward' to assign reviewers later.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			docID := args[0]

			c := requireAPI()
			result, err := c.SendReview(docID, &client.ReviewSendRequest{
				Reviewers: reviewers,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Review #%d created for %s (v%s)\n", result.ReviewID, docID, result.Version)
			if len(reviewers) > 0 {
				fmt.Printf("Reviewers: %s\n", strings.Join(reviewers, ", "))
			} else {
				fmt.Println("No reviewers assigned — use 'isms review forward' to assign when ready.")
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&reviewers, "to", nil, "Reviewer email (repeatable, optional)")
	return cmd
}

func reviewListCmd() *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List reviews",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := requireAPI()
			reviews, err := c.ListReviews(status)
			if err != nil {
				return err
			}
			if len(reviews) == 0 {
				fmt.Println("No reviews found.")
				return nil
			}
			fmt.Printf("  %-6s %-10s %-14s %-36s %-20s %s\n",
				"ID", "STATUS", "DOCUMENT", "TITLE", "REQUESTED BY", "DATE")
			fmt.Printf("  %s\n", strings.Repeat("-", 100))
			for _, r := range reviews {
				fmt.Printf("  %-6d %-10s %-14s %-36s %-20s %s\n",
					r.ID,
					r.Status,
					r.DocumentID,
					truncate(r.Title, 36),
					truncate(r.RequestedBy, 20),
					r.CreatedAt.Format("2006-01-02"),
				)
			}
			fmt.Printf("\n%d review(s)\n", len(reviews))
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (open, closed)")
	return cmd
}

func reviewShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <review-id>",
		Short: "Show review details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid review ID: %s", args[0])
			}

			c := requireAPI()
			review, err := c.GetReview(id)
			if err != nil {
				return err
			}
			fmt.Printf("Review #%d\n", review.ID)
			fmt.Printf("  Document:     %s\n", review.DocumentID)
			fmt.Printf("  Type:         %s\n", review.DocumentType)
			fmt.Printf("  Title:        %s\n", review.Title)
			fmt.Printf("  Version:      %s\n", review.Version)
			fmt.Printf("  Status:       %s\n", review.Status)
			fmt.Printf("  Requested by: %s\n", review.RequestedBy)
			fmt.Printf("  Created:      %s\n", review.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Printf("  Updated:      %s\n", review.UpdatedAt.Format("2006-01-02 15:04"))
			fmt.Printf("  Comments:     %d (%d open)\n", review.CommentCount, review.OpenComments)
			return nil
		},
	}
}

func reviewApproveCmd() *cobra.Command {
	var comment string

	cmd := &cobra.Command{
		Use:   "approve <review-id>",
		Short: "Approve a review (sets document to approved status)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid review ID: %s", args[0])
			}

			c := requireAPI()
			// The status endpoint only accepts "closed"; other transitions go
			// through dedicated handlers — approve via the real approval handler (#51).
			if err := c.ApproveReview(id, "approved", comment); err != nil {
				return err
			}
			fmt.Printf("Review #%d approved.\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&comment, "comment", "", "Approval comment")
	return cmd
}

func reviewAssignCmd() *cobra.Command {
	var (
		reviewers []string
		message   string
	)

	cmd := &cobra.Command{
		Use:   "assign <review-id>",
		Short: "Assign reviewers to a review",
		Long:  "Adds reviewers to an existing review. All assigned reviewers must approve before the review is complete.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid review ID: %s", args[0])
			}
			if len(reviewers) == 0 {
				return fmt.Errorf("at least one reviewer required (use --to)")
			}

			c := requireAPI()
			if err := c.ForwardReview(id, reviewers, message); err != nil {
				return err
			}
			fmt.Printf("Review #%d forwarded to %s\n", id, strings.Join(reviewers, ", "))
			if message != "" {
				fmt.Printf("Note: %s\n", message)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&reviewers, "to", nil, "Reviewer email (repeatable, required)")
	cmd.Flags().StringVar(&message, "message", "", "Optional note to the reviewers")
	return cmd
}

func reviewCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <review-id>",
		Short: "Close a review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid review ID: %s", args[0])
			}

			c := requireAPI()
			if err := c.UpdateReviewStatus(id, "closed"); err != nil {
				return err
			}
			fmt.Printf("Review #%d closed.\n", id)
			return nil
		},
	}
}
