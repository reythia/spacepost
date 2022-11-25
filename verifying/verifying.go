package verifying

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/spacemeshos/post/config"
	"github.com/spacemeshos/post/oracle"
	"github.com/spacemeshos/post/shared"
)

type (
	Config = config.Config
)

var (
	WorkOracle = oracle.WorkOracle
	FastOracle = oracle.FastOracle
	UInt64LE   = shared.UInt64LE
)

func VerifyPoW(nonce, numUnits, bitsPerLabel uint64, nodeId, atxId []byte) error {
	if len(nodeId) != 32 {
		return fmt.Errorf("invalid `nodeId` length; expected: 32, given: %v", len(nodeId))
	}

	if len(atxId) != 32 {
		return fmt.Errorf("invalid `atxId` length; expected: 32, given: %v", len(atxId))
	}

	numLabels := numUnits * bitsPerLabel
	difficulty := shared.PowDifficulty(numLabels)
	threshold := new(big.Int).SetBytes(difficulty)

	res, err := WorkOracle(
		oracle.WithNodeId(nodeId),
		oracle.WithAtxId(atxId),
		oracle.WithPosition(nonce),
		oracle.WithBitsPerLabel(uint32(bitsPerLabel)*32),
	)
	if err != nil {
		return err
	}

	label := new(big.Int).SetBytes(res.Output)
	if label.Cmp(threshold) > 0 {
		return fmt.Errorf("label is above the threshold; label: %#32x, threshold: %#32x", label, threshold)
	}

	return nil
}

// Verify ensures the validity of a proof in respect to its metadata.
// It returns nil if the proof is valid or an error describing the failure, otherwise.
func Verify(p *shared.Proof, m *shared.ProofMetadata) error {
	if len(m.NodeId) != 32 {
		return fmt.Errorf("invalid `nodeId` length; expected: 32, given: %v", len(m.NodeId))
	}

	if len(m.AtxId) != 32 {
		return fmt.Errorf("invalid `atxId` length; expected: 32, given: %v", len(m.AtxId))
	}

	numLabels := uint64(m.NumUnits) * uint64(m.LabelsPerUnit)
	bitsPerIndex := uint(shared.BinaryRepresentationMinBits(numLabels))
	expectedSize := shared.Size(bitsPerIndex, uint(m.K2))
	if expectedSize != uint(len(p.Indices)) {
		return fmt.Errorf("invalid indices set size; expected %d, given: %d", expectedSize, len(p.Indices))
	}

	difficulty := shared.ProvingDifficulty(numLabels, uint64(m.K1))
	buf := bytes.NewBuffer(p.Indices)
	gsReader := shared.NewGranSpecificReader(buf, bitsPerIndex)
	indicesSet := make(map[uint64]bool, m.K2)

	for i := uint(0); i < uint(m.K2); i++ {
		index, err := gsReader.ReadNextUintBE()
		if err != nil {
			return err
		}
		if indicesSet[index] {
			return fmt.Errorf("non-unique index: %d", index)
		}
		indicesSet[index] = true

		res, err := WorkOracle(
			oracle.WithNodeId(m.NodeId),
			oracle.WithAtxId(m.AtxId),
			oracle.WithPosition(index),
			oracle.WithBitsPerLabel(uint32(m.BitsPerLabel)),
		)
		if err != nil {
			return err
		}
		hash := FastOracle(m.Challenge, p.Nonce, res.Output)
		hashNum := UInt64LE(hash[:])
		if hashNum > difficulty {
			return fmt.Errorf("fast oracle output is above the threshold; index: %d, label: %x, hash: %x, hashNum: %d, difficulty: %d",
				index, res.Output, hash, hashNum, difficulty)
		}
	}

	return nil
}
