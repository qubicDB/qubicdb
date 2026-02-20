package sentiment

import (
	"math"
	"sync"

	"github.com/jonreiter/govader"
)

// Label represents one of the six universal basic emotions plus neutral.
// Mapping follows Ekman (1992) — the six emotions with universal facial expressions.
type Label string

const (
	LabelHappiness Label = "happiness"
	LabelSadness   Label = "sadness"
	LabelFear      Label = "fear"
	LabelAnger     Label = "anger"
	LabelDisgust   Label = "disgust"
	LabelSurprise  Label = "surprise"
	LabelNeutral   Label = "neutral"
)

// Result holds the full sentiment analysis output for a piece of text.
type Result struct {
	Label    Label   // Dominant basic emotion
	Compound float64 // VADER compound score [-1, 1]
	Positive float64 // VADER positive ratio [0, 1]
	Negative float64 // VADER negative ratio [0, 1]
	Neutral  float64 // VADER neutral ratio [0, 1]
}

// Analyzer wraps govader's SentimentIntensityAnalyzer and maps its output
// to the six basic emotions. It is safe for concurrent use.
type Analyzer struct {
	sia *govader.SentimentIntensityAnalyzer
	mu  sync.Mutex
}

var (
	defaultAnalyzer *Analyzer
	once            sync.Once
)

// Default returns the package-level singleton Analyzer (lazy-initialized).
func Default() *Analyzer {
	once.Do(func() {
		defaultAnalyzer = New()
	})
	return defaultAnalyzer
}

// New creates a new Analyzer. Prefer Default() for shared use.
func New() *Analyzer {
	return &Analyzer{
		sia: govader.NewSentimentIntensityAnalyzer(),
	}
}

// Analyze returns the sentiment Result for the given text.
// The Label is derived from VADER polarity scores using the mapping below:
//
//	compound >=  0.60  → happiness   (strong positive)
//	compound >=  0.20  → surprise    (mild positive — unexpected/arousing)
//	compound <= -0.60  → anger/disgust/fear (disambiguated by neg intensity)
//	compound <= -0.20  → sadness     (mild negative)
//	otherwise          → neutral
//
// Within the strong-negative band, the highest sub-score among neg/pos/neu
// is used to pick anger vs disgust vs fear heuristically.
func (a *Analyzer) Analyze(text string) Result {
	a.mu.Lock()
	scores := a.sia.PolarityScores(text)
	a.mu.Unlock()

	r := Result{
		Compound: scores.Compound,
		Positive: scores.Positive,
		Negative: scores.Negative,
		Neutral:  scores.Neutral,
	}
	r.Label = mapToLabel(scores.Compound, scores.Positive, scores.Negative, scores.Neutral)
	return r
}

// mapToLabel converts VADER scores to a basic emotion label.
func mapToLabel(compound, pos, neg, neu float64) Label {
	switch {
	case compound >= 0.60:
		return LabelHappiness
	case compound >= 0.20:
		return LabelSurprise
	case compound <= -0.60:
		return strongNegativeLabel(pos, neg, neu)
	case compound <= -0.20:
		return LabelSadness
	default:
		return LabelNeutral
	}
}

// strongNegativeLabel disambiguates anger / disgust / fear within the
// strong-negative band using the relative magnitude of the VADER sub-scores.
// Without word-level emotion lexicons we use a simple heuristic:
//
//	neg >> neu  → anger  (high arousal, confrontational)
//	neu > neg   → fear   (high uncertainty, avoidance)
//	balanced    → disgust (aversion without high arousal)
func strongNegativeLabel(pos, neg, neu float64) Label {
	_ = pos
	ratio := 0.0
	if neu > 0 {
		ratio = neg / neu
	} else {
		ratio = math.MaxFloat64
	}
	switch {
	case ratio > 1.5:
		return LabelAnger
	case neu > neg:
		return LabelFear
	default:
		return LabelDisgust
	}
}

// SentimentBoost returns a score multiplier [0.8, 1.2] that can be applied
// to search scoring when the query and neuron share the same emotional valence.
// Same label → boost (1.2), opposite valence → slight penalty (0.8), else neutral (1.0).
func SentimentBoost(queryLabel, neuronLabel Label) float64 {
	if queryLabel == LabelNeutral || neuronLabel == LabelNeutral {
		return 1.0
	}
	if queryLabel == neuronLabel {
		return 1.2
	}
	if isOppositeValence(queryLabel, neuronLabel) {
		return 0.8
	}
	return 1.0
}

// isOppositeValence returns true when one label is positive-valence and the
// other is negative-valence.
func isOppositeValence(a, b Label) bool {
	positive := map[Label]bool{LabelHappiness: true, LabelSurprise: true}
	negative := map[Label]bool{LabelSadness: true, LabelFear: true, LabelAnger: true, LabelDisgust: true}
	return (positive[a] && negative[b]) || (negative[a] && positive[b])
}
