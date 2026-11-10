package cli

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/activityforge/activityforge/internal/gitstore"
	"github.com/activityforge/activityforge/internal/render"
	"github.com/activityforge/activityforge/internal/web"
)

func (a *App) publish(ctx context.Context, o options) error {
	if o.offline {
		return fmt.Errorf("publication needs GitHub access; remove --offline")
	}
	if o.remote == "" {
		return fmt.Errorf("specify --remote owner/repository; create an empty standalone GitHub repository with main as default, or add --create-private")
	}
	p, err := a.loadSelected(o)
	if err != nil {
		return err
	}
	s := gitstore.Store{Repo: o.repo, State: o.state}
	prep, err := a.GitHub.Prepare(ctx, s, p, o.remote, o.createPrivate, true)
	if err != nil {
		return err
	}
	v := render.View{Plan: prep.Display, Kind: "publish", Frozen: true, Verification: &prep.Report, Target: o.remote, Pending: prep.Pending, CanPush: prep.Pending > 0, Notice: prep.Notice}
	if prep.Pending == 0 {
		fmt.Fprintln(a.Out, "Already published: remote matches verified local history; nothing to push.")
		v.Notice = "Remote already contains the verified commits. No push is needed."
	}
	if err = a.terminal(o, v.Plan); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Target: https://github.com/%s\nPending commits: %d\nVerified HEAD: %s\n", o.remote, prep.Pending, prep.Head)
	if err = a.export(o, v); err != nil {
		return err
	}
	if o.terminal {
		if prep.Pending == 0 {
			return nil
		}
		if prep.CreatePrivate {
			fmt.Fprintln(a.Out, "Approval will also create a PRIVATE GitHub repository.")
		}
		fmt.Fprintf(a.Out, "Type exactly 'push %s' to approve, or press Enter to cancel: ", o.remote)
		line, err := bufio.NewReader(a.In).ReadString('\n')
		if err != nil {
			return fmt.Errorf("publication cancelled: %w", err)
		}
		if strings.TrimSpace(line) != "push "+o.remote {
			fmt.Fprintln(a.Out, "Publication cancelled.")
			return nil
		}
		if err = a.GitHub.Push(ctx, prep); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Published. Enable GitHub profile → Contribution settings → Private contributions. GitHub indexing can take up to 24 hours.")
		return nil
	}
	opts := a.browserOptions(o, v)
	opts.Push = func(ctx context.Context) error { return a.GitHub.Push(ctx, prep) }
	return web.Serve(ctx, opts, o.port, !o.serve, a.Out)
}
