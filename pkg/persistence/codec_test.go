package persistence

import (
	"testing"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

func TestCodecEncodeDecodeWithCompression(t *testing.T) {
	codec := NewCodec(true)

	m := core.NewMatrix("test-user", core.DefaultBounds())
	n := core.NewNeuron("Test content", m.CurrentDim)
	m.Neurons[n.ID] = n

	// Encode
	data, err := codec.Encode(m)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Encoded data should not be empty")
	}

	// Decode
	decoded, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.IndexID != m.IndexID {
		t.Errorf("IndexID mismatch: expected %s, got %s", m.IndexID, decoded.IndexID)
	}
	if len(decoded.Neurons) != 1 {
		t.Errorf("Expected 1 neuron, got %d", len(decoded.Neurons))
	}
}

func TestCodecEncodeDecodeWithoutCompression(t *testing.T) {
	codec := NewCodec(false)

	m := core.NewMatrix("test-user", core.DefaultBounds())

	data, err := codec.Encode(m)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.IndexID != m.IndexID {
		t.Error("IndexID mismatch")
	}
}

func TestCodecMagicBytes(t *testing.T) {
	codec := NewCodec(false)

	m := core.NewMatrix("test-user", core.DefaultBounds())
	data, _ := codec.Encode(m)

	// Check magic bytes at start
	if string(data[:4]) != MagicBytes {
		t.Errorf("Expected magic bytes '%s', got '%s'", MagicBytes, string(data[:4]))
	}
}

func TestCodecInvalidData(t *testing.T) {
	codec := NewCodec(false)

	// Too short
	_, err := codec.Decode([]byte{1, 2, 3})
	if err == nil {
		t.Error("Should fail on too short data")
	}

	// Invalid magic
	invalidMagic := make([]byte, 100)
	copy(invalidMagic[:4], "XXXX")
	_, err = codec.Decode(invalidMagic)
	if err == nil {
		t.Error("Should fail on invalid magic bytes")
	}
}

func TestCreateSnapshot(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	n := core.NewNeuron("Test", m.CurrentDim)
	m.Neurons[n.ID] = n

	snap := CreateSnapshot(m)

	if snap.IndexID != m.IndexID {
		t.Error("IndexID mismatch")
	}
	if snap.NeuronCount != 1 {
		t.Errorf("Expected 1 neuron, got %d", snap.NeuronCount)
	}
	if snap.Version != m.Version {
		t.Error("Version mismatch")
	}
}

func TestSnapshotEncodeDecode(t *testing.T) {
	snap := Snapshot{
		IndexID:       "user-1",
		Version:      5,
		NeuronCount:  10,
		SynapseCount: 20,
		CurrentDim:   3,
		TotalEnergy:  8.5,
		ModifiedAt:   1234567890,
	}

	data, err := EncodeSnapshot(snap)
	if err != nil {
		t.Fatalf("EncodeSnapshot failed: %v", err)
	}

	decoded, err := DecodeSnapshot(data)
	if err != nil {
		t.Fatalf("DecodeSnapshot failed: %v", err)
	}

	if decoded.IndexID != snap.IndexID {
		t.Error("IndexID mismatch")
	}
	if decoded.NeuronCount != snap.NeuronCount {
		t.Error("NeuronCount mismatch")
	}
	if decoded.Version != snap.Version {
		t.Error("Version mismatch")
	}
}

func TestCodecWithLargeMatrix(t *testing.T) {
	codec := NewCodec(true)

	m := core.NewMatrix("test-user", core.DefaultBounds())

	// Add many neurons
	for i := 0; i < 100; i++ {
		n := core.NewNeuron("Test content number "+string(rune(i)), m.CurrentDim)
		m.Neurons[n.ID] = n
	}

	// Encode
	data, err := codec.Encode(m)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode
	decoded, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded.Neurons) != 100 {
		t.Errorf("Expected 100 neurons, got %d", len(decoded.Neurons))
	}
}
