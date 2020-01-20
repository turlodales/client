// Copyright 015 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package engine

import (
	"github.com/keybase/client/go/libkb"
	keybase1 "github.com/keybase/client/go/protocol/keybase1"
)

type ListTrackersUnverifiedEngine struct {
	libkb.Contextified
	arg keybase1.ListTrackersUnverifiedArg
	res keybase1.UserSummarySet
	uid keybase1.UID
}

func NewListTrackersUnverified(g *libkb.GlobalContext, arg keybase1.ListTrackersUnverifiedArg) *ListTrackersUnverifiedEngine {
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
	if len(e.arg.Assertion) == 0 {
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
	if len(e.arg.Assertion) == 0 {
		e.uid = m.G().GetMyUID()
		if !e.uid.Exists() {
			return libkb.NoUIDError{}
		}
		return nil
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
	ts := libkb.NewServertrustTracker2Syncer(m.G(), callerUID, libkb.FollowDirectionFollowers)
	if err := libkb.RunSyncer(m, ts, e.uid, false /* loggedIn */, false /* forceReload */); err != nil {
		return err
	}
	e.res = ts.Result()
	return nil
}

func (e *ListTrackersUnverifiedEngine) GetResults() keybase1.UserSummarySet {
	return e.res
}
