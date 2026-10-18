package git

import "context"

type PRResult struct {
	Opened int
	Merged int
}

func AggregatePRs(
	ctx context.Context, repo, since, until, ghToken string,
) (*PRResult, error) {
	return nil, nil
}
