package r6dissect

import "github.com/rs/zerolog/log"

type Scoreboard struct {
	Players []ScoreboardPlayer
}

type ScoreboardPlayer struct {
	ID               []byte
	Score            uint32
	Assists          uint32
	AssistsFromRound uint32
}

// this function fixes kills that were previously recorded as elims
func readScoreboardKills(r *Reader) error {
	kills, err := r.Uint32()
	if err != nil {
		return err
	}
	if err := r.Skip(30); err != nil {
		return err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return err
	}
	idx := r.PlayerIndexByID(id)
	if idx != -1 {
		username := r.Header.Players[idx].Username
		r.lastKillerFromScoreboard = username
		log.Warn().
			Str("username", username).
			Uint32("kills", kills).
			Msg("scoreboard_kill")
	}
	return nil
}

func readScoreboardAssists(r *Reader) error {
	assists, err := r.Uint32()
	if err != nil {
		return err
	}
	if assists == 0 {
		return nil
	}
	if err = r.Skip(30); err != nil {
		return err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return err
	}
	idx := r.PlayerIndexByID(id)
	username := "N/A"
	if idx != -1 {
		username = r.Header.Players[idx].Username
		r.Scoreboard.Players[idx].Assists = assists
		r.Scoreboard.Players[idx].AssistsFromRound++
	}
	log.Debug().
		Uint32("assists", assists).
		Str("username", username).
		Msg("scoreboard_assists")
	return nil
}

func readScoreboardScore(r *Reader) error {
	score, err := r.Uint32()
	if err != nil {
		return err
	}
	if score == 0 {
		return nil
	}
	if err = r.Skip(13); err != nil {
		return err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return err
	}
	idx := r.PlayerIndexByID(id)
	username := "N/A"
	if idx != -1 {
		username = r.Header.Players[idx].Username
		r.Scoreboard.Players[idx].Score = score
	}
	log.Debug().
		Uint32("score", score).
		Str("username", username).
		Msg("scoreboard_score")
	return nil
}
