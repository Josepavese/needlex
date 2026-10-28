package pipeline

import (
	"strings"
	"testing"
)

func TestReducerRetainsUncoveredTextInDOMOrder(t *testing.T) {
	dom, err := (Reducer{}).Reduce(RawPage{HTML: `<html><body>
<div>Introduzione <span>La conoscenza <em>resta condivisa</em> e accessibile.</span>
<small>Autrice: Sofia</small><x-record>知識は共有される。</x-record></div>
<section>Prima <p>Paragrafo <span>con enfasi</span>.</p> Dopo</section>
</body></html>`})
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	seen := map[string]bool{}
	for _, node := range dom.Nodes {
		if seen[node.Path] {
			t.Fatalf("duplicate source path %s", node.Path)
		}
		seen[node.Path] = true
		text = append(text, node.Text)
	}
	got := strings.Join(text, " ")
	want := "Introduzione La conoscenza resta condivisa e accessibile. Autrice: Sofia 知識は共有される。 Prima Paragrafo con enfasi . Dopo"
	if got != want {
		t.Fatalf("text coverage/order:\n got %q\nwant %q", got, want)
	}
	if !seen["/div[1]/span[1]/text()[1]"] || !seen["/section[1]/text()[2]"] {
		t.Fatalf("missing exact text-node source selectors: %v", seen)
	}
}

func TestReducerTextRecoveryRespectsVisibilityAndInertSubtrees(t *testing.T) {
	dom, err := (Reducer{}).Reduce(RawPage{HTML: `<body>
<div hidden>hidden wrapper</div><span aria-hidden="true">hidden inline</span>
<script>script payload</script><style>style payload</style><template>template payload<p>template paragraph</p></template>
<div>Visible data <p>Visible paragraph <span hidden>hidden descendant</span><b>visible inline</b></p></div>
<div class="cookie-banner">cookie payload</div>
</body>`})
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, node := range dom.Nodes {
		parts = append(parts, node.Text)
	}
	got := strings.Join(parts, " ")
	if got != "Visible data Visible paragraph visible inline" {
		t.Fatalf("unexpected visible content: %q", got)
	}
}
