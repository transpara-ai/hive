# Scout Report — Iteration 20

## Map (from code + state)

Read state.md. Mobile responsiveness complete (iter 19). Site functionally complete. State says "the product works — now it needs to breathe."

Explored lovyou2 animation patterns. Found a rich vocabulary:
- **Heart breathing**: 3s ease-in-out pulse (opacity 0.6→1, scale 1→1.1)
- **Scroll reveal**: IntersectionObserver + fade-up with staggered delays via CSS `--d` variable
- **Message appear**: 0.3s ease translateY(4px)→0
- **Thinking dots**: staggered opacity pulse
- **Progress transitions**: 0.5s ease width/opacity changes

Current site: zero animations. Every element appears instantly. The dark theme is polished but static.

## Gap Type

Missing refinement — the site has no motion.

## The Gap

The Ember Minimalism aesthetic defined in iterations 15-16 has no kinetic dimension. lovyou2's philosophy of "ritual minimalism" — slow, deliberate reveals, breathing brand elements — has not been carried forward. The site feels competent but lifeless.

## Why This Gap

Motion communicates intentionality. A breathing logo says "this is alive." Scroll reveals reward exploration. Staggered card appearances create rhythm. Without these, the dark theme feels flat rather than warm.

## Filled Looks Like

1. **Brand breathing** — subtle pulse on the lovyou.ai logo (opacity + slight scale, 3s ease-in-out infinite)
2. **Page reveal** — hero/heading elements fade up on page load with staggered delays
3. **Scroll reveal** — cards and sections fade up as they enter viewport (IntersectionObserver)
4. **Card hover glow** — subtle brand shadow on card hover (transition, already partially done with `hover:shadow-brand/5`)
5. All animations CSS-only except scroll reveal which needs a small IntersectionObserver script
