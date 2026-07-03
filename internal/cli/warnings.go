package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/cloud-forge/cli/internal/awsdeploy"
	"github.com/cloud-forge/cli/pkg/store"
)

func printDeployWarnings(w io.Writer, app *store.App, params map[string]string) {
	fmt.Fprintln(w, "Warning: This deploy creates billable AWS resources such as EC2 instances and Elastic IPs.")
	if app != nil && app.Price != "" && !strings.EqualFold(app.Price, "free") {
		fmt.Fprintf(w, "Warning: %s may incur AWS Marketplace software fees (%s) in addition to infrastructure costs.\n", app.Name, app.Price)
	}
	if allowedIP := params["AllowedIP"]; allowedIP == "0.0.0.0/0" || allowedIP == "" {
		fmt.Fprintln(w, "Warning: SSH is open to 0.0.0.0/0. Restrict access with --allowed-ip <your-ip>/32.")
	}
}

func printDeleteResult(w io.Writer, result *awsdeploy.DestroyOutput) {
	fmt.Fprintf(w, "AWS account: %s\n", result.AccountID)
	fmt.Fprintf(w, "AWS region:  %s\n", result.Region)
	fmt.Fprintf(w, "Action:      %s\n", result.Action)
	if result.StackName != "" {
		fmt.Fprintf(w, "Stack name:  %s\n", result.StackName)
	}
	if result.Status != "" {
		fmt.Fprintf(w, "Status:      %s\n", result.Status)
	}
}
