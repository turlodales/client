// Copyright 2015 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package engine

import (
	"errors"

	"github.com/keybase/client/go/libkb"
	keybase1 "github.com/keybase/client/go/protocol/keybase1"
)

type ListTrackersUnverifiedEngine struct {
	libkb.Contextified
	arg ListTrackersUnverifiedEngineArg
	res keybase1.UserSummarySet
	uid keybase1.UID
}

// If a UID is given, the engine will list its trackers
// If an Assertion is given, the engine will try to resolve it to a UID via
// remote unless CachedOnly is true.
// Otherwise, the logged-in uid is used.
// If no user is logged in, NoUIDError is returned.
type ListTrackersUnverifiedEngineArg struct {
	UID        keybase1.UID
	Assertion  string
	CachedOnly bool
}

func NewListTrackersUnverifiedEngine(g *libkb.GlobalContext, arg ListTrackersUnverifiedEngineArg) *ListTrackersUnverifiedEngine {
	return &ListTrackersUnverifiedEngine{
		Contextified: libkb.NewContextified(g),
		arg:          arg,
	}
}

// Name is the unique engine name.
func (e *ListTrackersUnverifiedEngine) Name() string {
	return "ListTrackersUnverifiedEngine"
}

// GetPrereqs returns the engine prereqs (none).
func (e *ListTrackersUnverifiedEngine) Prereqs() Prereqs {
	session := false
	if len(e.arg.Assertion) == 0 && e.arg.UID.IsNil() {
		session = true
	}
	return Prereqs{Device: session}
}

func (e *ListTrackersUnverifiedEngine) RequiredUIs() []libkb.UIKind {
	return []libkb.UIKind{libkb.LogUIKind}
}

func (e *ListTrackersUnverifiedEngine) SubConsumers() []libkb.UIConsumer {
	return nil
}

func (e *ListTrackersUnverifiedEngine) lookupUID(m libkb.MetaContext) error {
	if e.arg.UID.Exists() {
		e.uid = e.arg.UID
		return nil
	}
	if len(e.arg.Assertion) == 0 {
		e.uid = m.G().GetMyUID()
		if !e.uid.Exists() {
			return libkb.NoUIDError{}
		}
		return nil
	}

	if e.arg.CachedOnly {
		return errors.New("no uid passed and not logged in; cannot lookup assertion in CachedOnly mode")
	}

	larg := libkb.NewLoadUserArgWithMetaContext(m).WithPublicKeyOptional().WithName(e.arg.Assertion)
	u, err := libkb.LoadUser(larg)
	if err != nil {
		return err
	}
	e.uid = u.GetUID()
	return nil
}

func (e *ListTrackersUnverifiedEngine) Run(m libkb.MetaContext) error {
	if err := e.lookupUID(m); err != nil {
		return err
	}

	callerUID := m.G().Env.GetUID()
	ts := libkb.NewServertrustTrackerSyncer(m.G(), callerUID, libkb.FollowDirectionFollowers)

	if e.arg.CachedOnly {
		if err := libkb.RunSyncerCached(m, ts, e.uid); err != nil {
			return err
		}
		e.res = ts.Result()
		return nil
	}

	if err := libkb.RunSyncer(m, ts, e.uid, false /* loggedIn */, false /* forceReload */); err != nil {
		return err
	}
	e.res = ts.Result()
	return nil
}

func (e *ListTrackersUnverifiedEngine) GetResults() keybase1.UserSummarySet {
	return e.res
}
