package r6dissect

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"
)

type MatchUpdateType int

//go:generate stringer -type=MatchUpdateType
const (
	Kill MatchUpdateType = iota
	Death
	DefuserPlantStart
	DefuserPlantComplete
	DefuserDisableStart
	DefuserDisableComplete
	LocateObjective
	OperatorSwap
	Battleye
	PlayerLeave
	Other
)

type MatchUpdate struct {
	Type                   MatchUpdateType `json:"type"`
	Username               string          `json:"username,omitempty"`
	Target                 string          `json:"target,omitempty"`
	Headshot               *bool           `json:"headshot,omitempty"`
	Time                   string          `json:"time"`
	TimeInSeconds          float64         `json:"timeInSeconds"`
	Message                string          `json:"message,omitempty"`
	Operator               Operator        `json:"operator,omitempty"`
	usernameFromScoreboard string
}

func (i MatchUpdateType) MarshalJSON() (text []byte, err error) {
	return json.Marshal(stringerIntMarshal{
		Name: i.String(),
		ID:   int(i),
	})
}

func (i *MatchUpdateType) UnmarshalJSON(data []byte) (err error) {
	var x stringerIntMarshal
	if err = json.Unmarshal(data, &x); err != nil {
		return
	}
	*i = MatchUpdateType(x.ID)
	return
}

var activity2 = []byte{0x00, 0x00, 0x00, 0x22, 0xe3, 0x09, 0x00, 0x79}
var killIndicator = []byte{0x22, 0xd9, 0x13, 0x3c, 0xba}

func readMatchFeedback(r *Reader) error {
	if r.Header.CodeVersion >= Y9S1Update3 {
		if err := r.Skip(38); err != nil {
			return err
		}
	} else if r.Header.CodeVersion >= Y9S1 {
		if err := r.Skip(9); err != nil {
			return err
		}
		valid, err := r.Int()
		if err != nil {
			return err
		}
		if valid != 4 {
			return errors.New("match feedback failed valid check")
		}
		if err := r.Skip(24); err != nil {
			return err
		}
	} else {
		if err := r.Skip(1); err != nil {
			return err
		}
		if err := r.Seek(activity2); err != nil {
			return err
		}
	}
	size, err := r.Int()
	if err != nil {
		return err
	}
	if size == 0 { // kill or an unknown indicator at start of match
		killTrace, err := r.Bytes(5)
		if err != nil {
			return err
		}
		if !bytes.Equal(killTrace, killIndicator) {
			log.Debug().Hex("killTrace", killTrace).Send()
			return nil
		}
		username, err := r.String()
		if err != nil {
			return err
		}
		empty := len(username) == 0
		if empty {
			log.Debug().Str("warn", "kill username empty").Send()
		}
		// No idea what these 15 bytes mean (kill type?)
		if err = r.Skip(15); err != nil {
			return err
		}
		target, err := r.String()
		if err != nil {
			return err
		}
		if empty && len(target) > 0 {
			u := MatchUpdate{
				Type:          Death,
				Username:      target,
				Time:          r.timeRaw,
				TimeInSeconds: r.time,
			}
			r.MatchFeedback = append(r.MatchFeedback, u)
			log.Debug().Interface("match_update", u).Send()
			log.Debug().Msg("kill username empty because of death")
			return nil
		} else if empty {
			return nil
		}
		u := MatchUpdate{
			Type:          Kill,
			Username:      username,
			Target:        target,
			Time:          r.timeRaw,
			TimeInSeconds: r.time,
		}
		if err = r.Skip(56); err != nil {
			return err
		}
		headshot, err := r.Int()
		if err != nil {
			return err
		}
		headshotPtr := new(bool)
		if headshot == 1 {
			*headshotPtr = true
		}
		u.Headshot = headshotPtr
		// Ignore duplicates
		for _, val := range r.MatchFeedback {
			if val.Type == Kill && val.Username == u.Username && val.Target == u.Target {
				return nil
			}
		}
		// removing the elimination username for now
		if r.lastKillerFromScoreboard != username {
			u.usernameFromScoreboard = r.lastKillerFromScoreboard
		}
		r.MatchFeedback = append(r.MatchFeedback, u)
		log.Debug().Interface("match_update", u).Send()
		return nil
	}
	// TODO: Y9S1 may have removed or modified other match feedback options
	if r.Header.CodeVersion >= Y9S1 {
		return nil
	}
	b, err := r.Bytes(size)
	if err != nil {
		return err
	}
	msg := string(b)
	t := Other
	if strings.Contains(msg, "bombs") || strings.Contains(msg, "objective") {
		t = LocateObjective
	}
	if strings.Contains(msg, "BattlEye") {
		t = Battleye
	}
	if strings.Contains(msg, "left") {
		t = PlayerLeave
	}
	username := strings.Split(msg, " ")[0]
	if t == Other {
		username = ""
	} else {
		msg = ""
	}
	u := MatchUpdate{
		Type:          t,
		Username:      username,
		Target:        "",
		Time:          r.timeRaw,
		TimeInSeconds: r.time,
		Message:       msg,
	}
	r.MatchFeedback = append(r.MatchFeedback, u)
	log.Debug().Interface("match_update", u).Send()
	return nil
}
