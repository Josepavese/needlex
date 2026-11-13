package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// NetworkPolicy selects the network destinations that acquisition transports may
// reach. NetworkPolicyAny preserves the historical unrestricted transport.
type NetworkPolicy string

const (
	NetworkPolicyAny    NetworkPolicy = "any"
	NetworkPolicyPublic NetworkPolicy = "public"
)

// ParseNetworkPolicy resolves a configured policy name. Empty means "any".
func ParseNetworkPolicy(raw string) (NetworkPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(NetworkPolicyAny):
		return NetworkPolicyAny, nil
	case string(NetworkPolicyPublic):
		return NetworkPolicyPublic, nil
	default:
		return "", fmt.Errorf("network.policy must be one of %q or %q", NetworkPolicyAny, NetworkPolicyPublic)
	}
}

// ErrEgressDenied identifies a destination rejected by the network policy. The
// rejected operation has no less protected egress path; callers must report it,
// not retry it against an unprotected client.
var ErrEgressDenied = errors.New("egress denied by network policy")

const (
	EgressCodeScheme   = "unsupported_scheme"
	EgressCodeUserinfo = "url_credentials"
	EgressCodeHost     = "invalid_host"
	EgressCodeAddress  = "non_public_address"
)

// EgressDeniedError is the typed denial returned by policy enforcement.
type EgressDeniedError struct {
	Code    string
	Host    string
	Address string
	Reason  string
}

func (e *EgressDeniedError) Error() string {
	target := strings.TrimSpace(e.Host)
	if address := strings.TrimSpace(e.Address); address != "" {
		target = fmt.Sprintf("%s (%s)", target, address)
	}
	if target == "" {
		target = "destination"
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "destination is not allowed"
	}
	return fmt.Sprintf("%s: %s rejected: %s [code=%s]; no alternative egress path is available", ErrEgressDenied, target, reason, e.Code)
}

func (e *EgressDeniedError) Unwrap() error { return ErrEgressDenied }

// IsEgressDenied reports whether err was produced by the egress policy.
func IsEgressDenied(err error) bool { return errors.Is(err, ErrEgressDenied) }

// Resolver resolves hostnames for policy enforcement. Tests inject a resolver so
// no unit test needs live DNS.
type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// EgressDialFunc creates the validated TCP connection. Tests inject a dial that
// forwards to a local test server while recording the validated address.
type EgressDialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// EgressOptions overrides the policy's resolver or validated dialer.
type EgressOptions struct {
	Resolver Resolver
	Dial     EgressDialFunc
}

// Egress enforces a NetworkPolicy at connection time: DNS answers are validated
// and the validated address is dialed, so a second resolution cannot rebind the
// connection. A non-enforcing policy returns nil and leaves transports untouched.
type Egress struct {
	policy   NetworkPolicy
	resolver Resolver
	dial     EgressDialFunc
}

func NewEgress(policy NetworkPolicy, options ...EgressOptions) *Egress {
	if policy != NetworkPolicyPublic {
		return nil
	}
	var opts EgressOptions
	if len(options) != 0 {
		opts = options[0]
	}
	egress := &Egress{policy: policy, resolver: opts.Resolver, dial: opts.Dial}
	if egress.resolver == nil {
		egress.resolver = net.DefaultResolver
	}
	if egress.dial == nil {
		var dialer net.Dialer
		egress.dial = dialer.DialContext
	}
	return egress
}

func (e *Egress) Policy() NetworkPolicy {
	if e == nil {
		return NetworkPolicyAny
	}
	return e.policy
}

// ValidateURL rejects URLs the policy can never serve, before any connection.
func (e *Egress) ValidateURL(target *url.URL) error {
	if e == nil || target == nil {
		return nil
	}
	scheme := strings.ToLower(strings.TrimSpace(target.Scheme))
	if scheme != "http" && scheme != "https" {
		return &EgressDeniedError{Code: EgressCodeScheme, Host: target.Host, Reason: fmt.Sprintf("scheme %q is not http or https", target.Scheme)}
	}
	if target.User != nil {
		return &EgressDeniedError{Code: EgressCodeUserinfo, Host: target.Host, Reason: "URLs with embedded credentials are not allowed"}
	}
	host := strings.TrimSpace(target.Hostname())
	if host == "" {
		return &EgressDeniedError{Code: EgressCodeHost, Host: target.Host, Reason: "URL has no host"}
	}
	if address, ok := parseIPAddress(host); ok {
		return publicAddressOrDenial(address, host)
	}
	return nil
}

// ValidateURLString is ValidateURL for raw URLs (rendering and download entry points).
func (e *Egress) ValidateURLString(rawURL string) error {
	if e == nil {
		return nil
	}
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	return e.ValidateURL(target)
}

// ValidateRedirect guards req/v3 redirect hops; dials are validated regardless.
func (e *Egress) ValidateRedirect(target *url.URL) error { return e.ValidateURL(target) }

// HTTPClient clones base and enforces the policy on every request and redirect
// hop. Public mode never inherits environment proxies.
func (e *Egress) HTTPClient(base *http.Client) *http.Client {
	if e == nil {
		return base
	}
	client := http.Client{}
	if base != nil {
		client = *base
	}
	transport := newEgressTransport()
	if existing, ok := client.Transport.(*http.Transport); ok {
		transport = existing.Clone()
	}
	transport.Proxy = nil
	transport.DialContext = e.DialContext
	client.Transport = egressTransport{egress: e, base: transport}
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := e.ValidateURL(request.URL); err != nil {
			return err
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}

// newEgressTransport defaults to the standard transport without environment
// proxies and without relying on an unchecked type assertion.
func newEgressTransport() *http.Transport {
	if defaults, ok := http.DefaultTransport.(*http.Transport); ok {
		return defaults.Clone()
	}
	return &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// DialContext resolves, validates every answer and then dials a validated IP.
func (e *Egress) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if e == nil {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, address)
	}
	validated, err := e.validatedAddress(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return e.dial(ctx, network, validated)
}

type egressTransport struct {
	egress *Egress
	base   http.RoundTripper
}

func (t egressTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := t.egress.ValidateURL(request.URL); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(request)
}

func (e *Egress) validatedAddress(ctx context.Context, network, address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("egress address %q: %w", address, err)
	}
	if ip, ok := parseIPAddress(host); ok {
		if err := publicAddressOrDenial(ip, host); err != nil {
			return "", err
		}
		return net.JoinHostPort(ip.String(), port), nil
	}
	validated, err := e.validateHost(ctx, host)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(selectAddress(network, validated).String(), port), nil
}

// ValidateHost resolves host and validates every answer. Browser navigation
// cannot surface a rejected dial as a typed denial, so the initial render URL is
// validated before launch; the loopback proxy still enforces the actual dial.
func (e *Egress) ValidateHost(ctx context.Context, host string) error {
	if e == nil {
		return nil
	}
	_, err := e.validateHost(ctx, strings.TrimSpace(host))
	return err
}

// ValidateRenderURL validates a browser navigation URL and its resolved host.
func (e *Egress) ValidateRenderURL(ctx context.Context, rawURL string) error {
	if e == nil {
		return nil
	}
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if err := e.ValidateURL(target); err != nil {
		return err
	}
	return e.ValidateHost(ctx, target.Hostname())
}

func (e *Egress) validateHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, ok := parseIPAddress(host); ok {
		if err := publicAddressOrDenial(ip, host); err != nil {
			return nil, err
		}
		return []netip.Addr{ip}, nil
	}
	answers, err := e.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(answers) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses", host)
	}
	validated := make([]netip.Addr, 0, len(answers))
	for _, answer := range answers {
		ip, ok := netip.AddrFromSlice(answer.IP)
		if !ok {
			return nil, &EgressDeniedError{Code: EgressCodeAddress, Host: host, Reason: "resolver returned an invalid address"}
		}
		ip = ip.Unmap()
		if err := publicAddressOrDenial(ip, host); err != nil {
			return nil, err
		}
		validated = append(validated, ip)
	}
	return validated, nil
}

func selectAddress(network string, addresses []netip.Addr) netip.Addr {
	for _, address := range addresses {
		switch {
		case strings.HasSuffix(network, "4") && address.Is4():
			return address
		case strings.HasSuffix(network, "6") && address.Is6():
			return address
		}
	}
	return addresses[0]
}

func parseIPAddress(host string) (netip.Addr, bool) {
	address, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func publicAddressOrDenial(address netip.Addr, host string) error {
	if reason := nonPublicReason(address); reason != "" {
		return &EgressDeniedError{Code: EgressCodeAddress, Host: host, Address: address.String(), Reason: reason}
	}
	return nil
}

func nonPublicReason(address netip.Addr) string {
	if !address.IsValid() {
		return "invalid address"
	}
	switch {
	case address.IsUnspecified():
		return "address is unspecified"
	case address.IsLoopback():
		return "address is loopback"
	case address.IsPrivate():
		return "address is private"
	case address.IsLinkLocalUnicast():
		return "address is link-local"
	case address.IsLinkLocalMulticast():
		return "address is link-local multicast"
	case address.IsInterfaceLocalMulticast():
		return "address is interface-local multicast"
	case address.IsMulticast():
		return "address is multicast"
	case !address.IsGlobalUnicast():
		return "address is not globally routable"
	}
	for _, reserved := range nonPublicRanges {
		if reserved.prefix.Contains(address) {
			return reserved.reason
		}
	}
	return ""
}

type nonPublicRange struct {
	prefix netip.Prefix
	reason string
}

// nonPublicRanges covers ranges the net/netip classification helpers treat as
// global unicast but which are reserved, benchmarking, documentation,
// transition or carrier-grade NAT space.
var nonPublicRanges = []nonPublicRange{
	{netip.MustParsePrefix("0.0.0.0/8"), "address is in the unspecified network"},
	{netip.MustParsePrefix("100.64.0.0/10"), "address is carrier-grade NAT"},
	{netip.MustParsePrefix("192.0.0.0/24"), "address is an IETF protocol assignment"},
	{netip.MustParsePrefix("192.0.2.0/24"), "address is documentation space"},
	{netip.MustParsePrefix("192.88.99.0/24"), "address is 6to4 relay anycast"},
	{netip.MustParsePrefix("198.18.0.0/15"), "address is benchmarking space"},
	{netip.MustParsePrefix("198.51.100.0/24"), "address is documentation space"},
	{netip.MustParsePrefix("203.0.113.0/24"), "address is documentation space"},
	{netip.MustParsePrefix("240.0.0.0/4"), "address is reserved"},
	{netip.MustParsePrefix("255.255.255.255/32"), "address is broadcast"},
	{netip.MustParsePrefix("::/96"), "address is IPv4-compatible"},
	{netip.MustParsePrefix("64:ff9b::/96"), "address is a NAT64 transition range"},
	{netip.MustParsePrefix("64:ff9b:1::/48"), "address is a local-use NAT64 range"},
	{netip.MustParsePrefix("100::/64"), "address is discard-only space"},
	{netip.MustParsePrefix("2001::/32"), "address is a Teredo transition range"},
	{netip.MustParsePrefix("2001:2::/48"), "address is benchmarking space"},
	{netip.MustParsePrefix("2001:10::/28"), "address is an ORCHID range"},
	{netip.MustParsePrefix("2001:20::/28"), "address is an ORCHIDv2 range"},
	{netip.MustParsePrefix("2001:db8::/32"), "address is documentation space"},
	{netip.MustParsePrefix("2002::/16"), "address is a 6to4 transition range"},
	{netip.MustParsePrefix("3fff::/20"), "address is documentation space"},
	{netip.MustParsePrefix("fec0::/10"), "address is deprecated site-local space"},
}
