// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controlapi

import (
	"context"
	"fmt"

	"github.com/agent-substrate/substrate/cmd/ateapi/internal/controlapi/recovery"
	"github.com/agent-substrate/substrate/cmd/ateapi/internal/store"
	"github.com/agent-substrate/substrate/internal/resources"
	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
)

// actorRecoveryVisibility maps recovery.ClientVisibility onto the public
// ActorStatus.recovery_visibility enum. VisibilityNone and unknown values
// become NONE so clients never see an unspecified recovery signal after a
// Decide call.
func actorRecoveryVisibility(v recovery.ClientVisibility) ateapipb.ActorRecoveryVisibility {
	switch v {
	case recovery.RecoveryPending:
		return ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_PENDING
	case recovery.RecoverableFailure:
		return ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_RECOVERABLE_FAILURE
	case recovery.Unrecoverable:
		return ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_UNRECOVERABLE
	default:
		return ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_NONE
	}
}

// setStatusRecoveryVisibility writes the mapped visibility onto status.
func setStatusRecoveryVisibility(status *ateapipb.ActorStatus, v recovery.ClientVisibility) {
	if status == nil {
		return
	}
	status.RecoveryVisibility = actorRecoveryVisibility(v)
}

// persistActorRecoveryVisibility stores ClientVisibility on the Actor without
// changing lifecycle state. Callers that already mutate status in the same
// UpdateActor should use setStatusRecoveryVisibility instead.
func persistActorRecoveryVisibility(ctx context.Context, st crashActorStore, actorRef resources.ActorRef, actor *ateapipb.Actor, v recovery.ClientVisibility) error {
	if actor == nil {
		var err error
		actor, err = st.GetActor(ctx, actorRef)
		if err != nil {
			return fmt.Errorf("while loading actor to set recovery visibility: %w", err)
		}
	}
	want := actorRecoveryVisibility(v)
	if actor.GetStatus().GetRecoveryVisibility() == want {
		return nil
	}
	_, err := st.UpdateActor(ctx, actorRef, store.PreconditionFrom(actor), func(toUpdate *ateapipb.Actor) error {
		setStatusRecoveryVisibility(toUpdate.Status, v)
		return nil
	})
	if err != nil {
		return fmt.Errorf("while setting recovery visibility: %w", err)
	}
	return nil
}
