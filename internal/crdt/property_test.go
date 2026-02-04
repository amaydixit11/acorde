package crdt

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/google/uuid"
)

// Property: Commutativity
// A Merge B == B Merge A
func TestProperty_Commutativity(t *testing.T) {
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	t.Logf("Commutativity Seed: %d", seed)

	for i := 0; i < 50; i++ {
		rA := generateRandomReplica(rng)
		rB := generateRandomReplica(rng)

		// A Merge B
		aClone := rA.Clone()
		bClone := rB.Clone()
		aClone.Merge(bClone)

		// B Merge A
		aClone2 := rA.Clone()
		bClone2 := rB.Clone()
		bClone2.Merge(aClone2)

		if !replicasEqual(aClone, bClone2) {
			t.Errorf("Commutativity violation at iteration %d", i)
		}
	}
}

// Property: Idempotence
// A Merge A == A
func TestProperty_Idempotence(t *testing.T) {
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	t.Logf("Idempotence Seed: %d", seed)

	for i := 0; i < 50; i++ {
		r := generateRandomReplica(rng)
		rClone := r.Clone()

		r.Merge(rClone)

		if !replicasEqual(r, rClone) {
			t.Errorf("Idempotence violation at iteration %d", i)
		}
	}
}

// Property: Associativity
// (A Merge B) Merge C == A Merge (B Merge C)
func TestProperty_Associativity(t *testing.T) {
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	t.Logf("Associativity Seed: %d", seed)

	for i := 0; i < 50; i++ {
		rA := generateRandomReplica(rng)
		rB := generateRandomReplica(rng)
		rC := generateRandomReplica(rng)

		// Left: (A+B)+C
		leftA := rA.Clone()
		leftA.Merge(rB.Clone())
		leftA.Merge(rC.Clone())

		// Right: A+(B+C)
		rightBC := rB.Clone()
		rightBC.Merge(rC.Clone())
		rightA := rA.Clone()
		rightA.Merge(rightBC)

		if !replicasEqual(leftA, rightA) {
			t.Errorf("Associativity violation at iteration %d", i)
		}
	}
}

// Property: Convergence
// Given N replicas extending disjointly from a base, merging all to all should result in same state
func TestProperty_Convergence(t *testing.T) {
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	t.Logf("Convergence Seed: %d", seed)

	for i := 0; i < 20; i++ {
		numReplicas := 3 + rng.Intn(3) // 3-5 replicas
		replicas := make([]*Replica, numReplicas)

		// Base state
		base := generateRandomReplica(rng)
		for j := 0; j < numReplicas; j++ {
			replicas[j] = base.Clone()
			// Diverge
			applyRandomOps(replicas[j], rng, 5)
		}

		// Merge all into one "Master" using random order
		master := NewReplica(core.NewClock())
		perm := rng.Perm(numReplicas)
		for _, idx := range perm {
			master.Merge(replicas[idx])
		}

		// Verify that merging master back into any replica makes strict equality
		for j := 0; j < numReplicas; j++ {
			replicas[j].Merge(master)
			if !replicasEqual(replicas[j], master) {
				t.Errorf("Convergence violation: Replica %d != Master", j)
			}
		}
	}
}

// Helpers

func generateRandomReplica(rng *rand.Rand) *Replica {
	r := NewReplica(core.NewClock())
	applyRandomOps(r, rng, 10+rng.Intn(20))
	return r
}

func applyRandomOps(r *Replica, rng *rand.Rand, count int) {
	ids := r.entryIDs()

	for i := 0; i < count; i++ {
		op := rng.Intn(3) // 0=Add, 1=Update, 2=Delete
		
		if op == 0 || len(ids) == 0 {
			// Add
			id := uuid.New()
			r.AddEntryWithID(id, core.Note, randomBytes(rng), randomTags(rng))
			ids = append(ids, id)
		} else {
			// Update or Delete existing
			idx := rng.Intn(len(ids))
			id := ids[idx]

			if op == 1 {
				// Update
				content := randomBytes(rng)
				tags := randomTags(rng)
				r.UpdateEntry(id, &content, &tags)
			} else {
				// Delete
				r.DeleteEntry(id)
			}
		}
	}
}

func (r *Replica) entryIDs() []uuid.UUID {
	var ids []uuid.UUID
	for _, e := range r.ListEntries() {
		ids = append(ids, e.ID)
	}
	return ids
}

func randomBytes(rng *rand.Rand) []byte {
	size := 1 + rng.Intn(10)
	b := make([]byte, size)
	rng.Read(b)
	return b
}

func randomTags(rng *rand.Rand) []string {
	count := rng.Intn(4)
	tags := make([]string, count)
	pool := []string{"a", "b", "c", "d", "e"}
	for i := 0; i < count; i++ {
		tags[i] = pool[rng.Intn(len(pool))]
	}
	return tags
}

func replicasEqual(a, b *Replica) bool {
	stateA := a.State()
	stateB := b.State()

	// Compare Entries (LWW)
	// Order might differ? ListEntries sorts?
	// State().Entries is raw LWW elements. Not strictly sorted.
	// But our LWWSet.AllElements might not be deterministic order?
	// Let's rely on DeepEqual if order is deterministic. 
	// To be safe, we should essentially compare the SET CONTENT.
	
	mapA := make(map[uuid.UUID]LWWElement)
	for _, e := range stateA.Entries {
		mapA[e.Entry.ID] = e
	}
	mapB := make(map[uuid.UUID]LWWElement)
	for _, e := range stateB.Entries {
		mapB[e.Entry.ID] = e
	}

	if len(mapA) != len(mapB) {
		return false
	}
	for id, elA := range mapA {
		elB, ok := mapB[id]
		if !ok {
			return false
		}
		if !lwwElementEqual(elA, elB) {
			return false
		}
	}

	// Compare Tags
	// DeepEqual fails because map iteration order is random and TagSetState contains slices.
	// We need to compare specific fields independently or sort them.
	if len(stateA.Tags) != len(stateB.Tags) {
		return false
	}
	for id, tagStateA := range stateA.Tags {
		tagStateB, ok := stateB.Tags[id]
		if !ok {
			return false
		}
		if !tagSetStateEqual(tagStateA, tagStateB) {
			return false
		}
	}
	
	// Compare Clocks
	if stateA.ClockTime != stateB.ClockTime {
		return false
	}

	return true
}

func lwwElementEqual(a, b LWWElement) bool {
	// DeepEqual on struct usually works, provided byte slices are equal
	// Timestamp, Deleted, etc.
	return reflect.DeepEqual(a, b)
}

func tagSetStateEqual(a, b TagSetState) bool {
	return tokenSliceEqual(a.Adds, b.Adds) && tokenSliceEqual(a.Removes, b.Removes)
}

func tokenSliceEqual(a, b []TagToken) bool {
	if len(a) != len(b) {
		return false
	}
	// Convert to map for set comparison
	mA := make(map[TagToken]struct{})
	for _, t := range a {
		mA[t] = struct{}{}
	}
	for _, t := range b {
		if _, ok := mA[t]; !ok {
			return false
		}
	}
	return true
}
