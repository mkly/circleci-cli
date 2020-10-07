package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/CircleCI-Public/circleci-cli/api"
	"github.com/CircleCI-Public/circleci-cli/api/graphql"
)

type orbImportPlan struct {
	NewNamespaces           []string
	NewOrbs                 []api.Orb
	NewVersions             []api.OrbVersion
	AlreadyExistingVersions []api.OrbVersion
}

func importOrb(opts orbOptions) error {
	vs, err := versionsToImport(opts)
	if err != nil {
		return err
	}

	plan, err := generateImportPlan(opts, vs)
	if err != nil {
		return err
	}

	displayPlan(os.Stdout, plan)
	if !opts.noPrompt && !opts.tty.askUserToConfirm("Are you sure you would like to proceed?") {
		return nil
	}

	return applyPlan(opts, plan)
}

func versionsToImport(opts orbOptions) ([]api.OrbVersion, error) {
	cloudClient := graphql.NewClient("https://circleci.com", "graphql-unstable", "", opts.cfg.Debug)

	if opts.integrationTesting {
		cloudClient = opts.cl
	}

	var orbVersions []api.OrbVersion
	for _, ref := range opts.args {
		if !isNamespace(ref) {
			version, err := api.OrbInfo(cloudClient, ref)
			if err != nil {
				return nil, fmt.Errorf("orb info: %s", err.Error())
			}

			orbVersions = append(orbVersions, *version)
			continue
		}

		// TODO: support an `--all-versions` flag that gets all versions instead of latest version per orb?
		// Note: fetching all orb versions may not be possible. The best we could do is fetch an arbitrarily large number.
		// Otherwise, do some other operation that grabs orb source data from a single namespace.
		obv, err := api.ListNamespaceOrbVersions(cloudClient, ref)
		if err != nil {
			return nil, fmt.Errorf("list namespace orb versions: %s", err.Error())
		}

		orbVersions = append(orbVersions, obv...)
	}

	return orbVersions, nil
}

func generateImportPlan(opts orbOptions, orbVersions []api.OrbVersion) (orbImportPlan, error) {
	uniqueNamespaces := map[string]bool{}
	uniqueOrbs := map[string]api.Orb{}

	// Dedupe namespaces and orbs.
	for _, o := range orbVersions {
		ns, orb := o.Orb.Namespace, o.Orb.Name
		uniqueNamespaces[ns] = true
		// Use a {orb}:{namespace} key to address cases where orbs have identical names
		// across distinct namespaces.
		uniqueOrbs[orb+":"+ns] = o.Orb
	}

	var plan orbImportPlan
	for ns := range uniqueNamespaces {
		ok, err := api.NamespaceExists(opts.cl, ns) // TODO: this implementation will change to include the notion of "imported"
		if err != nil {
			return orbImportPlan{}, fmt.Errorf("namespace check failed: %s", err.Error())
		}

		if !ok {
			plan.NewNamespaces = append(plan.NewNamespaces, ns)
		}
	}

	for _, orb := range uniqueOrbs {
		ok, err := api.OrbExists(opts.cl, orb.Namespace, orb.Name)
		if err != nil {
			return orbImportPlan{}, fmt.Errorf("orb id check failed: %s", err.Error())
		}

		if !ok {
			plan.NewOrbs = append(plan.NewOrbs, orb)
		}
	}

	for _, o := range orbVersions {
		_, err := api.OrbInfo(opts.cl, fmt.Sprintf("%s/%s@%s", o.Orb.Namespace, o.Orb.Name, o.Version))
		if _, ok := err.(*api.ErrOrbVersionNotExists); ok {
			plan.NewVersions = append(plan.NewVersions, o)
			continue
		}
		if err != nil {
			return orbImportPlan{}, fmt.Errorf("orb info check failed: %s", err.Error())
		}

		plan.AlreadyExistingVersions = append(plan.AlreadyExistingVersions, o)
	}

	return plan, nil
}

func applyPlan(opts orbOptions, plan orbImportPlan) error {
	for _, ns := range plan.NewNamespaces {
		_, err := api.CreateImportedNamespace(opts.cl, ns)
		if err != nil {
			return fmt.Errorf("unable to create '%s' namespace: %s", ns, err.Error())
		}
	}

	for _, o := range plan.NewOrbs {
		_, err := api.CreateImportedOrb(opts.cl, o.Namespace, o.Name)
		if err != nil {
			return fmt.Errorf("unable to create '%s' orb: %s", o.Name, err.Error())
		}
	}

	for _, v := range plan.NewVersions {
		resp, err := api.OrbID(opts.cl, v.Orb.Namespace, v.Orb.Name)
		if err != nil {
			return fmt.Errorf("unable to get orb info at %s/%s: %s", v.Orb.Namespace, v.Orb.Name, err.Error())
		}

		_, err = api.OrbImportVersion(opts.cl, v.Source, resp.Orb.ID, v.Version)
		if err != nil {
			return fmt.Errorf("unable to publish '%s/%s@%s' with source: %s", v.Orb.Namespace, v.Orb.Name, v.Version, err.Error())
		}
	}

	return nil
}

func displayPlan(w io.Writer, plan orbImportPlan) {
	var b strings.Builder
	b.WriteString("The following actions will be performed:\n")

	for _, ns := range plan.NewNamespaces {
		b.WriteString(fmt.Sprintf("  Create namespace '%s'\n", ns))
	}

	for _, o := range plan.NewOrbs {
		b.WriteString(fmt.Sprintf("  Create orb '%s/%s'\n", o.Namespace, o.Name))
	}

	for _, v := range plan.NewVersions {
		b.WriteString(fmt.Sprintf("  Import version '%s/%s@%s'\n", v.Orb.Namespace, v.Orb.Name, v.Version))
	}

	for i, e := range plan.AlreadyExistingVersions {
		if i == 0 {
			b.WriteString("\nThe following orb versions already exist:\n")
		}
		b.WriteString(fmt.Sprintf("  ('%s/%s@%s')\n", e.Orb.Namespace, e.Orb.Name, e.Version))
	}

	fmt.Fprint(w, b.String())
}

func isNamespace(ref string) bool {
	return len(strings.Split(ref, "/")) == 1
}
