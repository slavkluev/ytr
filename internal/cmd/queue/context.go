package queue

import (
	"context"
	"sync"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/validate"
)

// QueueContextFields lists the parts of the queue context document, which are
// its top-level JSON keys and the names --json selects.
var QueueContextFields = []string{
	"key",
	"name",
	"defaultType",
	"defaultPriority",
	"issueTypes",
	"statuses",
	"workflows",
	"components",
	"requiredFields",
	"localFields",
	"globalFields",
	"incomplete",
}

const contextLong = `Show what an agent needs to create and move issues in a queue, as one JSON
document: issue types, statuses, workflow transitions, components, defaults,
required fields, local fields, and the editable global fields.

Without --json it is the whole document; --json selects parts and makes only
the requests they need.

key, name, defaultType, defaultPriority, and issueTypes come from the queue
request. statuses and workflows share the workflow requests, and each other
part has a request of its own. A part whose request failed is null, and
incomplete names it with the server's reason; a part that was fetched but is
empty is []. Once the queue itself is fetched the command exits 0, so check
incomplete before relying on a part: under --json, select incomplete beside the
parts, as in --json workflows,incomplete, to learn which selected parts are null
or may be missing entries.

PARTS
  key, name        the queue's key and name
  defaultType      default issue type key (what issue create --type takes)
  defaultPriority  default priority key (what issue create --priority takes)
  issueTypes       key, name, and the id of the workflow each type follows
  statuses         every status of the queue's workflows, once
  workflows        id, initialStatus, and transitions: each status key maps to
                   the status keys its actions lead to, [] for a status with
                   no outgoing transitions
  components       id and name
  requiredFields   summary, then every field the queue marks required; default
                   is the queue's default for type and priority. When Tracker
                   lists no queue fields, incomplete says so: other fields may
                   still be required
  localFields      id is the full <queueId>--<key> that --filter needs;
                   options are the allowed values
  globalFields     editable global fields, key and name only; ytr field get KEY
                   shows a field's schema and values
  incomplete       {part, reason} for each selected part that is null or may be
                   missing entries

JSON FIELDS
  key, name, defaultType, defaultPriority, issueTypes, statuses, workflows, components, requiredFields, localFields, globalFields, incomplete`

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context QUEUE-KEY",
		Short: "Show what is needed to create and move issues in a queue",
		Long:  contextLong,
		Example: `  # Everything needed to work in a queue
  ytr queue context PROJ

  # Only issue types and components (no workflow or field requests)
  ytr queue context PROJ --json issueTypes,components

  # Statuses an issue in "open" can move to
  ytr queue context PROJ --json workflows --jq '.workflows[].transitions.open'

  # Full local field ids for --filter
  ytr queue context PROJ --json localFields --jq '.localFields[] | {id, options}'`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, args []string) error {
			_, err := validate.ValidateStringID(args[0], "queue key")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			queueKey, _ := validate.ValidateStringID(args[0], "queue key")
			return runContext(cmd, queueKey)
		},
	}

	runner.SetFields(cmd, QueueContextFields)

	return cmd
}

func runContext(cmd *cobra.Command, queueKey string) error {
	opts, err := runner.SelectFields(cmd, QueueContextFields)
	if err != nil {
		return err
	}

	client, err := runner.Client(cmd)
	if err != nil {
		return err
	}

	// The queue comes first: without it there is no document, and its
	// issueTypesConfig names the workflows to fetch.
	q, _, err := client.Queues.Get(cmd.Context(), queueKey, &tracker.QueueGetOptions{Expand: "issueTypesConfig"})
	if err != nil {
		return api.MapAPIError(err)
	}

	results := fetchParts(cmd.Context(), client, queueKey, workflowIDs(q), wantedParts(opts.JSONFields))

	return runner.PrintJSON(cmd, opts, contextDocument(q, queueKey, results, opts.JSONFields))
}

// Each request runs in its own goroutine and writes only its own fields, which are
// read after all of them finish.
func fetchParts(
	ctx context.Context,
	client *tracker.Client,
	queueKey string,
	workflows []string,
	wanted map[string]bool,
) contextResults {
	var r contextResults
	var wg sync.WaitGroup

	if wanted[partStatuses] || wanted[partWorkflows] {
		wg.Go(func() {
			r.workflows, r.failedWorkflow, r.workflowsErr = fetchWorkflows(ctx, client.Workflows, workflows)
		})
	}
	if wanted[partComponents] {
		wg.Go(func() {
			opts := &tracker.QueueComponentsListOptions{Fields: "name"}
			r.components, _, r.componentsErr = client.Queues.ListComponents(ctx, queueKey, opts)
		})
	}
	if wanted[partRequiredFields] {
		wg.Go(func() {
			r.queueFields, _, r.queueFieldsErr = client.Queues.ListFields(ctx, queueKey)
		})
	}
	if wanted[partLocalFields] {
		wg.Go(func() {
			r.localFields, _, r.localFieldsErr = client.Fields.ListLocal(ctx, queueKey)
		})
	}
	if wanted[partGlobalFields] {
		wg.Go(func() {
			r.globalFields, _, r.globalErr = client.Fields.List(ctx)
		})
	}

	wg.Wait()
	return r
}

// fetchWorkflows fetches each workflow in order and stops at the first
// failure, returning the id that failed: one missing workflow leaves the
// statuses and transitions incomplete, so the rest would not be used.
func fetchWorkflows(
	ctx context.Context,
	workflowsService *tracker.WorkflowsService,
	ids []string,
) ([]*tracker.Workflow, string, error) {
	workflows := make([]*tracker.Workflow, 0, len(ids))
	for _, id := range ids {
		wf, _, err := workflowsService.Get(ctx, id)
		if err != nil {
			return nil, id, err
		}
		workflows = append(workflows, wf)
	}
	return workflows, "", nil
}
