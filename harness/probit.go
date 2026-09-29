package harness

import "math"

// lnPhi returns ln Φ(z), the log of the standard normal CDF, with full
// relative accuracy in the lower tail (math.Erfc underflows near z = −38,
// so below −35 it uses the asymptotic series of the Mills ratio).
func lnPhi(z float64) float64 {
	switch {
	case z < -35:
		z2 := z * z
		series := 1 - 1/z2 + 3/(z2*z2) - 15/(z2*z2*z2) + 105/(z2*z2*z2*z2)
		return -z2/2 - math.Log(-z) - 0.5*math.Log(2*math.Pi) + math.Log(series)
	case z < 0:
		return math.Log(0.5 * math.Erfc(-z/math.Sqrt2))
	default:
		return math.Log1p(-0.5 * math.Erfc(z/math.Sqrt2))
	}
}

// probitLog returns z such that ln Φ(z) = lp, for lp < 0. It works from the
// log so that tail probabilities far below the float64 range (an edge with
// thousands of events that goes to zero) still give a finite z.
func probitLog(lp float64) float64 {
	if lp >= 0 {
		return math.Inf(1)
	}
	if math.IsInf(lp, -1) {
		return math.Inf(-1)
	}
	if lp > math.Log(0.5) {
		// Upper half: use the complement, ln(1 − e^lp), and symmetry.
		return -probitLog(math.Log(-math.Expm1(lp)))
	}
	var z float64
	if p := math.Exp(lp); p > 1e-10 {
		z = -math.Sqrt2 * math.Erfcinv(2*p)
	} else {
		// Leading terms of the tail expansion of Φ⁻¹.
		t := -2 * lp
		z = -math.Sqrt(t - math.Log(t) - math.Log(2*math.Pi))
	}
	// Newton on f(z) = ln Φ(z) − lp; f is increasing and concave, so the
	// iterates converge monotonically after the first step.
	for i := 0; i < 50; i++ {
		lphi := lnPhi(z)
		lpdf := -z*z/2 - 0.5*math.Log(2*math.Pi)
		step := (lphi - lp) / math.Exp(lpdf-lphi)
		z -= step
		if math.Abs(step) < 1e-14*(1+math.Abs(z)) {
			break
		}
	}
	return z
}

// MidPZ returns the exact per-edge statistic for the two Poisson counts of
// SignedRoot: conditional on n = a + o, o ~ Binomial(n, q) under H0, and
//
//	r = Φ⁻¹(P(O < o) + P(O = o)/2)
//
// the normal quantile of the lower mid-p. r < 0 is silence, r > 0 is burst;
// −r = Φ⁻¹(upper mid-p), so the same r serves both one-sided tests. The
// smaller tail is summed in log space, so r stays finite for any n.
// n = 0 gives 0. a and o must be whole numbers.
func MidPZ(a, o, q float64) float64 {
	n := a + o
	if n == 0 {
		return 0
	}
	if o < n*q {
		return probitLog(midPTailLog(int(o), int(n), q, true))
	}
	return -probitLog(midPTailLog(int(o), int(n), q, false))
}

// midPTailLog returns ln[P(O < o) + P(O = o)/2] (lower) or
// ln[P(O > o) + P(O = o)/2] (upper) for O ~ Binomial(n, q). It sums away
// from o, which is the cheap direction when o is in that tail; terms are
// built from P(O = o) by the ratio of successive probabilities.
func midPTailLog(o, n int, q float64, lower bool) float64 {
	lgn, _ := math.Lgamma(float64(n + 1))
	lgk, _ := math.Lgamma(float64(o + 1))
	lgnk, _ := math.Lgamma(float64(n - o + 1))
	logPMF := lgn - lgk - lgnk
	if o > 0 {
		logPMF += float64(o) * math.Log(q)
	}
	if n > o {
		logPMF += float64(n-o) * math.Log1p(-q)
	}
	odds := q / (1 - q) // P(k+1)/P(k) = (n−k)/(k+1) · odds
	sum, term := 0.5, 1.0
	if lower {
		for k := o; k > 0; k-- {
			term *= float64(k) / float64(n-k+1) / odds
			sum += term
			if term < 1e-17*sum {
				break
			}
		}
	} else {
		for k := o; k < n; k++ {
			term *= float64(n-k) / float64(k+1) * odds
			sum += term
			if term < 1e-17*sum {
				break
			}
		}
	}
	return logPMF + math.Log(sum)
}
