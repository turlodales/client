package service

import (
	"errors"
	"fmt"
	"sync"

	"github.com/keybase/client/go/engine"
	"github.com/keybase/client/go/libkb"
	"github.com/keybase/client/go/protocol/keybase1"
	context "golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
)

type trackerLoaderContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type TrackerLoader struct {
	libkb.Contextified
	sync.Mutex

	eg         errgroup.Group
	ctx        *trackerLoaderContext
	started    bool
	shutdownCh chan struct{}
	queueCh    chan keybase1.UID
}

func NewTrackerLoader(g *libkb.GlobalContext) *TrackerLoader {
	l := &TrackerLoader{
		Contextified: libkb.NewContextified(g),
		shutdownCh:   make(chan struct{}),
		queueCh:      make(chan keybase1.UID, 100),
	}

	g.PushShutdownHook(func(mctx libkb.MetaContext) error {
		<-l.Shutdown(mctx.Ctx())
		return nil
	})
	return l
}

func (l *TrackerLoader) debug(ctx context.Context, msg string, args ...interface{}) {
	l.G().Log.CDebugf(ctx, "TrackerLoader: %s", fmt.Sprintf(msg, args...))
}

func (l *TrackerLoader) Run(ctx context.Context) {
	defer l.G().CTrace(ctx, "TrackerLoader.Run", func() error { return nil })()
	l.Lock()
	defer l.Unlock()
	if l.started {
		return
	}
	l.started = true
	l.shutdownCh = make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)
	l.ctx = &trackerLoaderContext{ctx: ctx, cancel: cancel}
	l.eg.Go(func() error { return l.loadLoop(l.ctx.ctx, l.shutdownCh) })
}

func (l *TrackerLoader) Shutdown(ctx context.Context) chan struct{} {
	defer l.G().CTrace(ctx, "TrackerLoader.Shutdown", func() error { return nil })()
	l.Lock()
	defer l.Unlock()
	ch := make(chan struct{})
	if l.started {
		if l.ctx != nil {
			l.ctx.cancel()
		}
		close(l.shutdownCh)
		l.started = false
		go func() {
			_ = l.eg.Wait()
			close(ch)
		}()
	} else {
		close(ch)
	}
	return ch
}

func (l *TrackerLoader) Queue(ctx context.Context, uid keybase1.UID) (err error) {
	defer l.G().CTrace(ctx, fmt.Sprintf("TrackerLoader.Queue: uid: %s", uid), func() error { return err })()
	select {
	case l.queueCh <- uid:
	default:
		return errors.New("queue full")
	}
	return nil
}

func (l *TrackerLoader) trackingArg(mctx libkb.MetaContext, uid keybase1.UID, withNetwork bool) *engine.ListTrackingEngineArg {
	loadUserArg := libkb.NewLoadUserArgWithMetaContext(mctx).WithUID(uid).WithCachedOnly(!withNetwork)
	return &engine.ListTrackingEngineArg{LoadUserArg: &loadUserArg}
}

func (l *TrackerLoader) trackersArg(uid keybase1.UID, withNetwork bool) engine.ListTrackersUnverifiedEngineArg {
	return engine.ListTrackersUnverifiedEngineArg{UID: uid, CachedOnly: !withNetwork}
}

func (l *TrackerLoader) loadInner(mctx libkb.MetaContext, uid keybase1.UID, withNetwork bool) error {
	eng := engine.NewListTrackingEngine(mctx.G(), l.trackingArg(mctx, uid, withNetwork))
	err := engine.RunEngine2(mctx, eng)
	if err != nil {
		return err
	}
	following := eng.TableResult()

	eng2 := engine.NewListTrackersUnverifiedEngine(mctx.G(), l.trackersArg(uid, withNetwork))
	err = engine.RunEngine2(mctx, eng2)
	if err != nil {
		return err
	}
	followers := eng2.GetResults()

	l.G().NotifyRouter.HandleTrackingInfo(keybase1.TrackingInfoArg{
		Uid:       uid,
		Followees: following.Usernames(),
		Followers: followers.Usernames(),
	})

	return nil
}

func (l *TrackerLoader) load(ctx context.Context, uid keybase1.UID) error {
	defer l.G().CTraceTimed(ctx, "TrackerLoader.load", func() error { return nil })()

	mctx := libkb.NewMetaContext(ctx, l.G())

	withNetwork := false
	if err := l.loadInner(mctx, uid, withNetwork); err != nil {
		l.debug(ctx, "load: failed to load from local storage: %s", err)
	}

	withNetwork = true
	return l.loadInner(mctx, uid, withNetwork)
}

func (l *TrackerLoader) loadLoop(ctx context.Context, stopCh chan struct{}) error {
	for {
		select {
		case uid := <-l.queueCh:
			if err := l.load(ctx, uid); err != nil {
				l.debug(ctx, "loadLoop: failed to load: %s", err)
			}
		case <-stopCh:
			return nil
		}
	}
}
