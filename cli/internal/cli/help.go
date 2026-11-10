package cli

import "fmt"

func (a *App) help() {
	fmt.Fprintln(a.Out, `Activity Forge — decorative GitHub activity, planned locally.

Usage:
  activityforge                         Interactive setup and adjustment wizard
  activityforge init                    Save initial JSON configuration
  activityforge configure               Edit configuration interactively
  activityforge preview --open          Interactive browser preview on 127.0.0.1
  activityforge plan --output plan.json Freeze the exact timestamps and settings
  activityforge generate --plan plan.json
  activityforge inspect --open          Verify real Git log and view actual history
  activityforge publish --remote owner/repo
  activityforge status                  Read manifest and verify current history

Common options:
  --state-dir PATH   Separate config, cache, plans and manifests (env ACTIVITYFORGE_STATE_DIR)
  --repo PATH        Dedicated empty repository (default STATE/history)
  --config PATH      Load a JSON config; flags override it
  --plan PATH        Use immutable plan; refuses generation-setting overrides
  --days 365         Last N days ending today, inclusive
  --start YYYY-MM-DD --end YYYY-MM-DD --timezone Europe/Moscow
  --preset balanced|active-developer|hardcore|chaotic|custom
  --intensity 1..10 --randomness 0..100 --seed 42
  --mean N --min N --max N --inactive PERCENT
  --weekends off|rare|light|moderate|same|custom
  --weekend-probability 0..1 --saturday-multiplier N --sunday-max N
  --bursts=false --rest=false --streaks=false --monthly=false
  --vacations N --vacation-min N --vacation-max N
  --trend stable|increasing|decreasing|wave|random
  --work-start 09:00 --work-end 18:00 --evening 0.12 --night 0.025
  --time-spread 35 --as-of RFC3339|now
  --author-name NAME --author-email EMAIL
  --fetch [--github-user LOGIN]          Fetch real GraphQL calendar with gh
  --offline                             No GitHub network calls
  --no-existing                         Ignore existing activity and cache
  --existing snapshot.json              Use a chosen cached GitHub Snapshot
  --open | --serve [--port 0]            Browser preview; Ctrl+C stops server
  --output PATH --format html|svg|png|json|csv
  --no-color --width 100                 Terminal rendering controls
  --create-private                      Offer private remote creation on approval
  --terminal                            Typed approval instead of browser publish

GitHub login is optional for local preview. gh auth login enables fetching/publishing.
Without gh, GH_TOKEN or GITHUB_TOKEN supports the GitHub API and normal HTTPS push.
No global Git config changes, history rewrites, force pushes or automatic publication.`)
}
