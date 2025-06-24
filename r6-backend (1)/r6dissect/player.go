package r6dissect

import (
	"bytes"
	"strings"

	"github.com/rs/zerolog/log"
)

func readPlayer(r *Reader) error {
	idIndicator := []byte{0x33, 0xD8, 0x3D, 0x4F, 0x23}
	if r.Header.CodeVersion <= Y7S2 {
		idIndicator = []byte{0xE6, 0xF9, 0x7D, 0x86}
	}
	spawnIndicator := []byte{0xAF, 0x98, 0x99, 0xCA}
	profileIDIndicator := []byte{0x8A, 0x50, 0x9B, 0xD0}
	//unknownIndicator := []byte{0x22, 0xEE, 0xD4, 0x45, 0xC8, 0x08} // maybe player appearance?
	r.playersRead++
	defer func() {
		if r.playersRead == 10 {
			r.deriveTeamRoles()
		}
	}()
	username, err := r.String()
	if err != nil {
		return err
	}
	if r.Header.CodeVersion >= Y7S4 {
		if err := r.Seek([]byte{0x40, 0xF2, 0x15, 0x04}); err != nil {
			return err
		}
		if err = r.Skip(8); err != nil {
			return err
		}
		swap, err := r.Bytes(1)
		if err != nil {
			return err
		}
		// Sometimes, 0x40, 0xF2, 0x15, 0x04 is sent twice.
		// Does not seem to be linked to role swap.
		if swap[0] == 0x9D {
			return nil
		}
	} else {
		if err := r.Seek([]byte{0x22, 0xA9, 0x26, 0x0B, 0xE4}); err != nil {
			return err
		}
	}
	op, err := r.Uint64() // Op before atk role swaps
	if err != nil {
		return err
	}
	if op == 0 { // Empty player slot
		log.Debug().Msg("empty player slot?")
		return nil
	}
	validPlayer, err := r.Bytes(1)
	if err != nil {
		return err
	}
	if validPlayer[0] != 0x22 {
		log.Warn().Uint64("op", op).Msg("strange invalid player located")
		return nil
	}
	if err := r.Seek(idIndicator); err != nil {
		return err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return err
	}
	if err := r.Seek(spawnIndicator); err != nil {
		return err
	}
	spawn, err := r.String()
	if err != nil {
		return err
	}
	if spawn == "" {
		if err = r.Skip(10); err != nil {
			return err
		}
		valid, err := r.Bytes(1)
		if err != nil {
			return err
		}
		if !bytes.Equal(valid, []byte{0x1B}) {
			return nil
		}
	}
	teamIndex := 0
	if r.playersRead > 5 {
		teamIndex = 1
	}
	// ui id (y9s3+?)
	// there seems to be more to this, but its a quick fix for atk op swaps for now
	var uiID uint64
	if r.Header.CodeVersion >= Y9S3 {
		if err = r.Seek([]byte{0x38, 0xDF, 0xEE, 0x88}); err != nil {
			return err
		}
		if err = r.Skip(13); err != nil {
			return err
		}
		if uiID, err = r.Uint64(); err != nil {
			return err
		}
	}
	// Older versions of siege did not include profile ids
	profileID := ""
	var unknownId uint64
	if len(r.Header.RecordingProfileID) > 0 {
		if err = r.Seek(profileIDIndicator); err != nil {
			return err
		}
		profileID, err = r.String()
		if err != nil {
			return err
		}
		if err = r.Skip(5); err != nil { // 22eed445c8
			return err
		}
		unknownId, err = r.Uint64()
		if err != nil {
			return err
		}
	} else {
		log.Debug().Str("warn", "profileID not found, skipping").Send()
	}
	p := Player{
		ID:        unknownId,
		ProfileID: profileID,
		Username:  username,
		TeamIndex: teamIndex,
		Operator:  Operator(op),
		Spawn:     spawn,
		DissectID: id,
		uiID:      uiID,
	}
	if p.Operator != Recruit && p.Operator.Role() == Defense {
		p.Spawn = r.Header.Site // We cannot detect the spawn here on defense
	}
	log.Debug().Str("username", username).
		Int("teamIndex", teamIndex).
		Interface("op", p.Operator).
		Str("profileID", profileID).
		Hex("DissectID", id).
		Uint64("ID", p.ID).
		Uint64("uiID", p.uiID).
		Str("spawn", spawn).Send()
	found := false
	for i, existing := range r.Header.Players {
		if existing.Username == p.Username ||
			(r.Header.CodeVersion < Y8S2 && existing.ID == p.ID && p.ID != 0) ||
			(r.Header.CodeVersion >= Y8S2 && bytes.Equal(existing.DissectID, p.DissectID)) ||
			(r.Header.CodeVersion <= Y7S2 && strings.HasPrefix(p.Username, existing.Username)) {
			r.Header.Players[i].ProfileID = p.ProfileID
			r.Header.Players[i].Username = p.Username
			r.Header.Players[i].Operator = p.Operator
			r.Header.Players[i].Spawn = p.Spawn
			r.Header.Players[i].DissectID = p.DissectID
			r.Header.Players[i].uiID = p.uiID
			found = true
			break
		}
	}
	if !found && len(username) > 0 {
		r.Header.Players = append(r.Header.Players, p)
	}
	return err
}

func readAtkOpSwap(r *Reader) error {
	op, err := r.Uint64()
	if err != nil {
		return err
	}
	o := Operator(op)
	// before Y9S3 caster view overhaul
	if r.Header.CodeVersion < Y9S3 {
		if err = r.Skip(5); err != nil {
			return err
		}
		id, err := r.Bytes(4)
		if err != nil {
			return err
		}
		i := r.PlayerIndexByID(id)
		log.Debug().Hex("id", id).Interface("op", op).Msg("atk_op_swap")
		if i > -1 {
			r.Header.Players[i].Operator = o
			u := MatchUpdate{
				Type:          OperatorSwap,
				Username:      r.Header.Players[i].Username,
				Time:          r.timeRaw,
				TimeInSeconds: r.time,
				Operator:      o,
			}
			r.MatchFeedback = append(r.MatchFeedback, u)
			log.Debug().Interface("match_update", u).Send()
		}
		return nil
	}
	// after Y9S3 caster view overhaul
	if err = r.Skip(402); err != nil {
		return err
	}
	// id shows up in player data and in op swaps afaik
	id, err := r.Uint64()
	if err != nil {
		return err
	}
	for i, p := range r.Header.Players {
		if p.uiID == id {
			r.Header.Players[i].Operator = o
			u := MatchUpdate{
				Type:          OperatorSwap,
				Username:      p.Username,
				Time:          r.timeRaw,
				TimeInSeconds: r.time,
				Operator:      o,
			}
			r.MatchFeedback = append(r.MatchFeedback, u)
			log.Debug().Interface("match_update", u).Send()
			break
		}
	}
	return nil
}
