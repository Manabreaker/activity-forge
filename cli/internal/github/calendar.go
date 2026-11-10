package github

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/activityforge/activityforge/internal/forge"
)

const calendarQuery = `query($login:String!,$from:DateTime!,$to:DateTime!){user(login:$login){contributionsCollection(from:$from,to:$to){contributionCalendar{totalContributions colors weeks{contributionDays{date weekday contributionCount contributionLevel color}}}}}}`

type apiCalendar struct {
	Data struct {
		User *struct {
			Collection struct {
				Calendar struct {
					Total  int      `json:"totalContributions"`
					Colors []string `json:"colors"`
					Weeks  []struct {
						Days []struct {
							forge.Contribution
							Date string `json:"date"`
						} `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
			} `json:"contributionsCollection"`
		} `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Fetch uses <=300-day windows, below GitHub's one-year GraphQL limit.
func (c Client) Fetch(ctx context.Context, start, end, login string) (forge.Snapshot, error) {
	s := forge.Snapshot{Start: start, End: end, Days: map[string]forge.Contribution{}}
	a, err := time.Parse(time.DateOnly, start)
	if err != nil {
		return s, err
	}
	b, err := time.Parse(time.DateOnly, end)
	if err != nil {
		return s, err
	}
	if b.Before(a) || b.Sub(a).Hours()/24 > forge.MaxDays {
		return s, fmt.Errorf("invalid GitHub date range")
	}
	if login == "" {
		account, err := c.Account(ctx)
		if err != nil {
			return s, err
		}
		login = account.Login
	}
	s.Login = login
	for cursor := a; !cursor.After(b); {
		last := cursor.AddDate(0, 0, 299)
		if last.After(b) {
			last = b
		}
		data, err := c.call(ctx, "gh", "api", "graphql", "-f", "query="+calendarQuery, "-f", "login="+login, "-f", "from="+cursor.Format(time.RFC3339), "-f", "to="+last.AddDate(0, 0, 1).Add(-time.Second).Format(time.RFC3339))
		if err != nil {
			return s, err
		}
		var response apiCalendar
		if err = json.Unmarshal(data, &response); err != nil {
			return s, err
		}
		if len(response.Errors) > 0 {
			return s, fmt.Errorf("GitHub GraphQL: %s", response.Errors[0].Message)
		}
		if response.Data.User == nil {
			return s, fmt.Errorf("GitHub user %q not found", login)
		}
		cal := response.Data.User.Collection.Calendar
		s.Colors = cal.Colors
		for _, week := range cal.Weeks {
			for _, day := range week.Days {
				if day.Date >= start && day.Date <= end {
					s.Days[day.Date] = day.Contribution
				}
			}
		}
		cursor = last.AddDate(0, 0, 1)
	}
	s.Known = true
	s.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	for _, day := range s.Days {
		s.TotalContributions += day.Count
	}
	return s, forge.ValidateSnapshot(s, start, end)
}
