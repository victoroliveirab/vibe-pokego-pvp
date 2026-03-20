package logging

// Config holds runtime settings needed to build a logger.
type Config struct {
	Env                 string
	BetterstackToken    string
	BetterstackEndpoint string
}
