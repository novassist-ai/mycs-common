//go:build linux

package network

import (
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
)

type packetFilterRouter struct {
	nft nftables.Conn

	table  []*nftables.Table
	chains [][]*nftables.Chain

	forwardChainCtPos []uint64

	ipDenyList, ipAllowList []*nftables.Set

	inboundIFNameVmap []*nftables.Set
	inboundChains map[string][]*nftables.Chain

	sgPortVmaps map[string]*nftables.Set

	setMap  map[string]*nftables.Set
	ruleMap map[string][]*nftables.Rule
	ruleRef map[string]int
}

const (
	filterPreRoute = iota
	filterInput
	filterForward
	filterOutput
	natPreRoute
	natPostRoute
)

var (
	// negative offset will give chains created for 
	// this router a high priority compared chains 
	// in the kernal with default priorities
	NftChainPriorityOffset int32 = 0

	ipSetElemTypes = []nftables.SetDatatype{ nftables.TypeIPAddr, nftables.TypeIP6Addr }
)

func (m *routeManager) NewFilterRouter(denyAll bool) (FilterRouter, error) {

	var (
		err error

		fwPolicy *nftables.ChainPolicy
		
		forwardRules []*nftables.Rule
	)

	r := &packetFilterRouter{
		table:  make([]*nftables.Table, 2),
		chains: make([][]*nftables.Chain, 2),

		forwardChainCtPos: make([]uint64, 2),

		ipDenyList:  make([]*nftables.Set, 2),
		ipAllowList: make([]*nftables.Set, 2),

		inboundIFNameVmap: make([]*nftables.Set, 2),
		inboundChains:     make(map[string][]*nftables.Chain),
		sgPortVmaps:       make(map[string]*nftables.Set),

		ruleMap: make(map[string][]*nftables.Rule),
		ruleRef: make(map[string]int),
	}

	r.table[0] = r.nft.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   "mycs_router_ipv4",
	})
	r.table[1] = r.nft.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv6,
		Name:   "mycs_router_ipv6",
	})
	r.chains[0] = make([]*nftables.Chain, 6)
	r.chains[1] = make([]*nftables.Chain, 6)

	// sets chain priority based on given default
	var chainPriorityRef = func(p *nftables.ChainPriority) *nftables.ChainPriority {
		cp := nftables.ChainPriority(int32(*p) + NftChainPriorityOffset)
		return &cp
	}
	// NOTE: policy ref for policy constants missing in nftables package
	var chainPolicyRef = func (p nftables.ChainPolicy) *nftables.ChainPolicy {
		return &p
	}
	if denyAll {
		fwPolicy = chainPolicyRef(nftables.ChainPolicyDrop)
	} else {
		fwPolicy = chainPolicyRef(nftables.ChainPolicyAccept)
	}

	for i, table := range r.table {

		// map ctstate {
		//   type ct_state : verdict
		//   elements = { invalid : drop, established : accept, related : accept }
		// }
		ctstateVmap := &nftables.Set{
			Name:     "ctstate",
			Table:    table,
			IsMap:    true,
			KeyType:  nftables.TypeCTState,
			DataType: nftables.TypeVerdict,
		}
		if err = r.nft.AddSet(ctstateVmap,
			[]nftables.SetElement{
				{
					Key:         binaryutil.NativeEndian.PutUint32(expr.CtStateBitINVALID),
					VerdictData: &expr.Verdict{ Kind: expr.VerdictDrop },
				},
				{
					Key:         binaryutil.NativeEndian.PutUint32(expr.CtStateBitESTABLISHED),
					VerdictData: &expr.Verdict{ Kind: expr.VerdictAccept },
				},
				{
					Key:         binaryutil.NativeEndian.PutUint32(expr.CtStateBitRELATED),
					VerdictData: &expr.Verdict{ Kind: expr.VerdictAccept },
				},
			},
		); err != nil {
			return nil, err
		}
		ctstateExpr := []expr.Any{
			// [ ct load status => reg 1 ]
			&expr.Ct{
				Register:       1,
				SourceRegister: false,
				Key:            expr.CtKeySTATE,
			},
			// [ lookup reg 1 map ctstateVmap ]
			&expr.Lookup{
				SourceRegister: 1,
				SetName:        ctstateVmap.Name,
				SetID:          ctstateVmap.ID,
				DestRegister:   0,
				IsDestRegSet:   true,
			},
		}

		// set ip_denylist
		r.ipDenyList[i] = &nftables.Set{
			Name:     "ip_denylist",
			Table:    table,
			KeyType:  ipSetElemTypes[i],
		}
		if err = r.nft.AddSet(r.ipDenyList[i], nil); err != nil {
			return nil, err
		}

		// set ip_allowlist
		r.ipAllowList[i] = &nftables.Set{
			Name:     "ip_allowlist",
			Table:    table,
			KeyType:  ipSetElemTypes[i],
		}
		if err = r.nft.AddSet(r.ipAllowList[i], nil); err != nil {
			return nil, err
		}

		// map inbound_ifname {
		//   type iifname : verdict
		//   elements = { "lo" : accept }
		// }
		//
		// NOTE: there appears to be an issue when
		//       iifname verdict map is created using
		//       the nftables library where the key
		//       is not being echoed when rules are
		//       listed via the cli. however, the
		//       settings seem to take effect and
		//       behave as expected.
		//
		r.inboundIFNameVmap[i] = &nftables.Set{
			Name:     "inbound_ifname",
			Table:    table,
			IsMap:    true,
			KeyType:  nftables.TypeIFName,
			DataType: nftables.TypeVerdict,
		}
		if err = r.nft.AddSet(r.inboundIFNameVmap[i],
			// ensure loop back traffic is allowed
			[]nftables.SetElement{
				{
					Key:         byteString("lo", 16),
					VerdictData: &expr.Verdict{ Kind: expr.VerdictAccept },
				},
			},
		); err != nil {
			return nil, err
		}
		inboundIFNameExpr := []expr.Any{
			// [ meta load iifname => reg 1 ]
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			// [ lookup reg 1 map inboundIFNameVmap ]
			&expr.Lookup{
				SourceRegister: 1,
				SetName:        r.inboundIFNameVmap[i].Name,
				SetID:          r.inboundIFNameVmap[i].ID,
				DestRegister:   0,
				IsDestRegSet:   true,
			},
		}

		// chains
		r.chains[i][filterPreRoute] = r.nft.AddChain(&nftables.Chain{
			Name:     "prerouting",
			Table:    table,
			Hooknum:  nftables.ChainHookPrerouting,
			Priority: chainPriorityRef(nftables.ChainPriorityFilter),
			Type:     nftables.ChainTypeFilter,
			Policy:   chainPolicyRef(nftables.ChainPolicyAccept),
		})
		r.chains[i][filterInput] = r.nft.AddChain(&nftables.Chain{
			Name:     "input",
			Table:    table,
			Hooknum:  nftables.ChainHookInput,
			Priority: chainPriorityRef(nftables.ChainPriorityFilter),
			Type:     nftables.ChainTypeFilter,
			Policy:   fwPolicy,
		})
		r.chains[i][filterForward] = r.nft.AddChain(&nftables.Chain{
			Name:     "forward",
			Table:    table,
			Hooknum:  nftables.ChainHookForward,
			Priority: chainPriorityRef(nftables.ChainPriorityFilter),
			Type:     nftables.ChainTypeFilter,
			Policy:   fwPolicy,
		})
		r.chains[i][filterOutput] = r.nft.AddChain(&nftables.Chain{
			Name:     "output",
			Table:    table,
			Hooknum:  nftables.ChainHookOutput,
			Priority: chainPriorityRef(nftables.ChainPriorityFilter),
			Type:     nftables.ChainTypeFilter,
			Policy:   chainPolicyRef(nftables.ChainPolicyAccept),
		})
		r.chains[i][natPreRoute] = r.nft.AddChain(&nftables.Chain{
			Name:     "nat_prerouting",
			Table:    table,
			Hooknum:  nftables.ChainHookPrerouting,
			Priority: chainPriorityRef(nftables.ChainPriorityNATDest),
			Type:     nftables.ChainTypeNAT,
			Policy:   chainPolicyRef(nftables.ChainPolicyAccept),
		})
		r.chains[i][natPostRoute] = r.nft.AddChain(&nftables.Chain{
			Name:     "nat_postrouting",
			Table:    table,
			Hooknum:  nftables.ChainHookPostrouting,
			Priority: chainPriorityRef(nftables.ChainPriorityNATSource),
			Type:     nftables.ChainTypeNAT,
			Policy:   chainPolicyRef(nftables.ChainPolicyAccept),
		})

		// chain input
		//
		// ct state vmap @ctstate
		r.nft.AddRule(&nftables.Rule{
			Table: table,
			Chain: r.chains[i][filterInput],
			Exprs: ctstateExpr,
		})
		// iifname vmap @inbound_ifname
		r.nft.AddRule(&nftables.Rule{
			Table: table,
			Chain: r.chains[i][filterInput],
			Exprs: inboundIFNameExpr,
		})

		// chain forward
		//
		// ct state vmap @ctstate
		r.nft.AddRule(&nftables.Rule{
			Table: table,
			Chain: r.chains[i][filterForward],
			Exprs: ctstateExpr,
		})
	}

	if err = r.nft.Flush(); err != nil {
		return nil, err
	}
	// retrieve handle for ct state rule for forward chain. all 
	// forward security groups will be inserted before this rule.
	for i, table := range r.table {
		if forwardRules, err = r.nft.GetRules(table, r.chains[i][filterForward]); err != nil {
			return nil, err
		}
		r.forwardChainCtPos[i] = forwardRules[0].Handle
	}

	m.pfr = r
	return m.pfr, nil
}

func (r *packetFilterRouter) getTable(
	isIPv4 bool,
) (
	*nftables.Table,
	[]*nftables.Chain,
	error,
) {

	if r.table == nil {
		return nil, nil,
			fmt.Errorf("packet filter router has not been initialized")
	}
	if isIPv4 {
		return r.table[0], r.chains[0], nil
	} else {
		return r.table[1], r.chains[1], nil
	}
}

func (r *packetFilterRouter) getChain(chain int) []*nftables.Chain {
	return []*nftables.Chain{ r.chains[0][chain], r.chains[1][chain] }
}

func (r *packetFilterRouter) getInboundChain(ifname string) ([]*nftables.Chain, error) {

	var (
		err error
		ok  bool

		inboundChain []*nftables.Chain
	)

	chainName := "inbound_"+ifname
	if inboundChain, ok = r.inboundChains[chainName]; !ok {

		logger.TraceMessage("packetFilterRouter.getInboundChain(): Creating inbound chain '%s' and adding jump condition.", chainName)

		inboundChain = make([]*nftables.Chain, len(r.table))
		for i, table := range r.table {
			inboundChain[i] = r.nft.AddChain(&nftables.Chain{
				Name:  chainName,
				Table: table,
			})
			if err = r.nft.SetAddElements(r.inboundIFNameVmap[i],
				[]nftables.SetElement{
					{
						Key:         byteString(ifname, 16),
						VerdictData: &expr.Verdict{
							Kind: expr.VerdictJump,
							Chain: chainName,
						},
					},
				},
			); err != nil {
				return nil, err
			}
		}
		r.inboundChains[chainName] = inboundChain
	}
	return inboundChain, nil
}

func (r *packetFilterRouter) AddIPsToDenyList(ips []netip.Addr) error {

	var (
		err error
		ok  bool

		rules []*nftables.Rule
	)

	if len(ips) > 0 {
		if err = r.addIPSetElements(r.ipDenyList, ips); err != nil {
			return err
		}

		if _, ok = r.ruleMap["ip_denylist"]; !ok {
			// if denylist lookup rule is not set then set it
			for i, chain := range r.getChain(filterPreRoute) {
				addrLen, srcOffset, _, _ := ipHeaderOffsets(i == 0)
	
				rules = append(rules, 
					// ip saddr @ip_denylist drop
					&nftables.Rule{
						Table: chain.Table,
						Chain: chain,
						Exprs: []expr.Any{
							// [ payload load 4b @ network header + 12 (src addr) => reg 1 ]
							&expr.Payload{
								DestRegister: 1,
								Base:         expr.PayloadBaseNetworkHeader,
								Offset:       srcOffset,
								Len:          addrLen,
							},
							// [ lookup reg 1 set whitelist ]
							&expr.Lookup{
								SourceRegister: 1,
								SetName:        r.ipDenyList[i].Name,
							},
							//[ immediate reg 0 drop ]
							&expr.Verdict{
								Kind: expr.VerdictDrop,
							},
						},
					},
				)
			}
			err = r.saveFilterRules("ip_denylist", rules, false)
		}
	}
	return err
}

func (r *packetFilterRouter) DeleteIPsFromDenyList(ips []netip.Addr) error {

	var (
		err  error
		size []int
	)

	if size, err = r.deleteIPSetElements(r.ipDenyList, ips); err == nil && size[0] == 0 && size[1] == 0 {
		err = r.DeleteFilter("ip_denylist")
	}
	return err
}

func (r *packetFilterRouter) AddIPsToAllowList(ips []netip.Addr) error {

	var (
		err error
		ok  bool

		rules []*nftables.Rule
	)

	if len(ips) > 0 {
		if err = r.addIPSetElements(r.ipAllowList, ips); err != nil {
			return err
		}

		if _, ok = r.ruleMap["ip_allowlist"]; !ok {
			// if allowlist lookup rule is not set then set it
			for i, chain := range r.getChain(filterPreRoute) {
				addrLen, srcOffset, _, _ := ipHeaderOffsets(i == 0)

				rules = append(rules, 
					[]*nftables.Rule{
						// iifname lo accept
						&nftables.Rule{
							Table: chain.Table,
							Chain: chain,
							Exprs: []expr.Any{
								// [ meta load iifname => reg 1 ]
								&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
								// [ cmp eq reg 1 lo ]
								&expr.Cmp{
									Op:       expr.CmpOpEq,
									Register: 1,
									Data:     []byte("lo\x00"),
								},
								//[ immediate reg 0 accept ]
								&expr.Verdict{
									Kind: expr.VerdictAccept,
								},
							},
						},
						// ip saddr != @ip_allowlist drop
						&nftables.Rule{
							Table: chain.Table,
							Chain: chain,
							Exprs: []expr.Any{
								// [ payload load 4b @ network header + 12 (src addr) => reg 1 ]
								&expr.Payload{
									DestRegister: 1,
									Base:         expr.PayloadBaseNetworkHeader,
									Offset:       srcOffset,
									Len:          addrLen,
								},
								// [ lookup reg 1 set whitelist ]
								&expr.Lookup{
									SourceRegister: 1,
									SetName:        r.ipAllowList[i].Name,
									Invert:         true,
								},
								//[ immediate reg 0 drop ]
								&expr.Verdict{
									Kind: expr.VerdictDrop,
								},
							},
						},
					}...,
				)
			}
			err = r.saveFilterRules("ip_allowlist", rules, false)
		}
	}
	return nil
}

func (r *packetFilterRouter) DeleteIPsFromAllowList(ips []netip.Addr) error {

	var (
		err  error
		size []int
	)

	if size, err = r.deleteIPSetElements(r.ipAllowList, ips); err == nil && size[0] == 0 && size[1] == 0 {
		err = r.DeleteFilter("ip_allowlist")
	}
	return err
}

// add ips to an ip set pair
func (r *packetFilterRouter) addIPSetElements(ipSet []*nftables.Set, ips []netip.Addr) error {

	var (
		err error
	)

	ipSetElems := createIPSetElements(ips)
	for i, setElems := range ipSetElems {
		if r.nft.SetAddElements(ipSet[i], setElems); err != nil {
			return err
		}
	}
	return nil
}

// delete ips from an ip set pair and returns the size of the sets after deletion
func (r *packetFilterRouter) deleteIPSetElements(ipSet []*nftables.Set, ips []netip.Addr) ([]int, error) {

	var (
		err error

		elems []nftables.SetElement
	)

	ipSetElems := createIPSetElements(ips)
	for i, setElems := range ipSetElems {
		if r.nft.SetDeleteElements(ipSet[i], setElems); err != nil {
			return nil, err
		}
	}
	// commit changes
	if err = r.nft.Flush(); err != nil {
		return nil, err
	}
	// retrieve size of sets
	size := []int{ 0, 0 }
	for i, set := range ipSet {
		if elems, err = r.nft.GetSetElements(set); err != nil {
			logger.ErrorMessage(
				"packetFilterRouter.deleteIPSetElements(): Failed to get list of elements for set '%s': %s",
				set.Name, err.Error(),
			)
			continue
		}
		size[i] = len(elems)
	}
	return size, nil
}

func (r *packetFilterRouter) SetSecurityGroups(sgs []SecurityGroup, iifName string) error {

	// chain inbound_<itf_name>
	//
	// sgPortVmaps =>
	//
	// map sgs_<itf_name> {
	//   type proto . dport : verdict
	// }
	// map sgs_<itf_name>_<src_network> {
	//   type proto . dport : verdict
	// }
	// map sgs_<itf_name>_<dst_network>_<src_network> {
	//   type proto . dport : verdict
	// }

	// chain forward
	//
	// sgPortVmaps =>
	//
	// map sgs_<src_network> {
	//   type proto . dport : verdict
	// }
	//
	// map sgs_<dst_network> {
	//   type proto . dport : verdict
	// }
	//
	// map sgs_<dst_network>_<src_network> {
	//   type proto . dport : verdict
	// }

	var (
		err error

		sgKey      string
		pgVmapName []string

		keyData    []byte
		sgPortVmap *nftables.Set

		verdict expr.VerdictKind
		rules   []*nftables.Rule
	)
	nullPos := []uint64{0, 0}

	for _, sg := range sgs {
		logger.TraceMessage("packetFilterRouter.SetSecurityGroups(): Applying security group: %# v", sg)

		// validate sg
		if sg.SrcNetwork.IsValid() && sg.DstNetwork.IsValid() && sg.SrcNetwork.Addr().Is4() != sg.DstNetwork.Addr().Is4() {
			logger.ErrorMessage("packetFilterRouter.SetSecurityGroups(): Cannot mix ipv4 and ipv6 types for SrcNetwork and DstNetwork: %# v", sg)
			continue
		}
		// get sg keys
		if sgKey, pgVmapName, err = sg.CreateSecurityGroupKeys(iifName); err != nil {
			logger.ErrorMessage("packetFilterRouter.SetSecurityGroups(): Error creating security group keys: %s", err.Error())
			continue
		}

		rules = nil
		sgExprsPre := []expr.Any{}

		// For tcp and udp protocols create rule that binds to port group vmap
		//
		// iifname <iifName> ip saddr <sg.SrcNetwork> ip daddr <sg.DstNetwork> ip protocol . dport vmap @<sgPortVmap>
		//
		// For icmp protocol create rule with accept or deny
		//
		// iifname <iifName> ip saddr <sg.SrcNetwork> ip daddr <sg.DstNetwork> icmp type echo-request accept

		// add filter to forward chain unless
		// otherwise determined below
		targetChain := r.getChain(filterForward)
		insertPos := r.forwardChainCtPos
		insert := true

		if len(iifName) > 0 {
			if !sg.DstNetwork.IsValid() {
				// if interface name is set and no dst network then
				// sg filter will be added to the inbound chain for
				// that interface
				if targetChain, err = r.getInboundChain(iifName); err != nil {
					logger.ErrorMessage("packetFilterRouter.SetSecurityGroups(): Failed to get/create inbound chain for interface '%s': %s", iifName, err.Error())
					continue
				}
				insertPos = nullPos
				insert = false

			} else {
				sgExprsPre = append(sgExprsPre,
					// [ meta load iifname => reg 1 ]
					&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
					// [ cmp eq reg 1 <iifname> ]
					&expr.Cmp{
						Op:       expr.CmpOpEq,
						Register: 1,
						Data:     []byte(iifName+"\x00"),
					},
				)
			}

		} else if !sg.DstNetwork.IsValid() {
			// no dst network so add filter to the input chain
			targetChain = r.getChain(filterInput)
			insertPos = nullPos
			insert = false
		}

		// apply rules to both ip4 and ipv6 chains
		// unless otherwise determined below
		addToChain := []bool{ true, true }
		if sg.SrcNetwork.IsValid() || sg.DstNetwork.IsValid() {
			addToChain[0] = sg.SrcNetwork.Addr().Is4() || sg.DstNetwork.Addr().Is4() // apply rules to ip4 chain
			addToChain[1] = sg.SrcNetwork.Addr().Is6() || sg.DstNetwork.Addr().Is6() // apply rules to ip6 chain
		}

		// set rule verdict for security group
		if sg.Deny {
			verdict = expr.VerdictDrop
		} else {
			verdict = expr.VerdictAccept
		}

		for i, chain := range targetChain {
			sgExprs := sgExprsPre

			if addToChain[i] {
				sgPortVmap = nil
				addICPMRule, addVMapPortRule := sync.Once{}, sync.Once{}
				addrLen, srcOffset, destOffset, protoOffset := ipHeaderOffsets(i == 0)

				if sg.SrcNetwork.IsValid() {
					srcNetworkMask := net.CIDRMask(sg.SrcNetwork.Bits(), int(addrLen)*8)
					sgExprs = append(sgExprs,
						// [ payload load 4b @ network header + 12 (src addr) => reg 1 ]
						&expr.Payload{
							DestRegister: 1,
							Base:         expr.PayloadBaseNetworkHeader,
							Offset:       srcOffset,
							Len:          addrLen,
						},
						// [ bitwise reg 1 = (reg=1 & <srcNetwork Mask> ) ^ 0x00000000 ]
						&expr.Bitwise{
							SourceRegister: 1,
							DestRegister:   1,
							Len:            addrLen,
							Mask:           []byte(srcNetworkMask),
							Xor:            make([]byte, addrLen),
						},
						// [ cmp eq reg 1 <srcNetwork in canonical form> ]
						&expr.Cmp{
							Op:       expr.CmpOpEq,
							Register: 1,
							Data:     sg.SrcNetwork.Masked().Addr().AsSlice(),
						},
					)
				}
				if len(sg.Oifname) > 0 {
					oifname := []byte(sg.Oifname+"\x00")
					sgExprs = append(sgExprs,
						// [ meta load oifname => reg 1 ]
						&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
						// [ cmp eq reg 1 <oifname> ]
						&expr.Cmp{
							Op:       expr.CmpOpEq,
							Register: 1,
							Data:     oifname,
						},
					)
				}
				if sg.DstNetwork.IsValid() {
					dstNetworkMask := net.CIDRMask(sg.DstNetwork.Bits(), int(addrLen)*8)
					sgExprs = append(sgExprs,
						// [ payload load 4b @ network header + 16 (dest addr) => reg 1 ]
						&expr.Payload{
							DestRegister: 1,
							Base:         expr.PayloadBaseNetworkHeader,
							Offset:       destOffset,
							Len:          addrLen,
						},
						// [ bitwise reg 1 = (reg=1 & dstNetwork Mask> ) ^ 0x00000000 ]
						&expr.Bitwise{
							SourceRegister: 1,
							DestRegister:   1,
							Len:            addrLen,
							Mask:           []byte(dstNetworkMask),
							Xor:            make([]byte, addrLen),
						},
						// [ cmp eq reg 1 <dstNetwork in canonical form> ]
						&expr.Cmp{
							Op:       expr.CmpOpEq,
							Register: 1,
							Data:     sg.DstNetwork.Masked().Addr().AsSlice(),
						},
					)
				}

				if len(sg.Ports) > 0 {
					// add port filter rules to vmap
					for _, pg := range sg.Ports {

						if pg.Proto == ICMP {
							// rule added only once for all port groups. so
							// any additional port groups with icmp protocol
							// will be ignored as icmp is a special case
							addICPMRule.Do(func(){
								// icmp needs to be handled as a seperate pool
								// from port lookup as icmp packets do not have
								// the a transport header port field
								rules = append(rules,
									&nftables.Rule{
										Table: chain.Table,
										Chain: chain,
										Position: insertPos[i],
										Exprs: append(sgExprs,
											// [ meta load l4proto => reg 1 ]
											&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
											// [ cmp eq reg 1 <protoData> ]
											&expr.Cmp{
												Op:       expr.CmpOpEq,
												Register: 1,
												Data:     []byte{unix.IPPROTO_ICMP},
											},
											// [ immediate reg 0 drop/accept ]
											&expr.Verdict{
												Kind: verdict,
											},
										),
									},
								)
							})
	
						} else {
							// rule added only once for all port groups
							addVMapPortRule.Do(func(){
								if sgPortVmap, err = r.getSecurityGroupPortVMap(pgVmapName[i], chain.Table); err == nil {
									// port filters are added to the port vmap so
									// the a vmap lookup binding needs to be created
									rules = append(rules,
										&nftables.Rule{
											Table: chain.Table,
											Chain: chain,
											Position: insertPos[i],
											Exprs: append(sgExprs,
												// [ payload load 1b @ network header + <protoOffset> => reg 9 ]
												&expr.Payload{
													Len:          1,
													Base:         expr.PayloadBaseNetworkHeader,
													Offset:       protoOffset, // Protocol IPv4 / NextHdr IPv6
													DestRegister: 9,
												},
												// [ payload load 2b @ transport header + 2 (dest port) => reg 10 ]
												&expr.Payload{
													Len:          2,
													Base:         expr.PayloadBaseTransportHeader,
													Offset:       2, // Destination Port
													DestRegister: 10,
												},
												// [ lookup reg 1 map <sgPortVmap> ]
												&expr.Lookup{
													SourceRegister: 9,
													SetName:        sgPortVmap.Name,
													DestRegister:   0,
													IsDestRegSet:   true,
												},
											),
										},
									)
									
								} else {
									logger.ErrorMessage(
										"packetFilterRouter.SetSecurityGroups(): Failed to retrieve security group port vmap with name '%s' for table '%s': %s",
										 pgVmapName[i], chain.Table.Name, err.Error(),
									)
								}
							})
							if sgPortVmap == nil {
								continue
							}
							for p := pg.FromPort; p <= pg.ToPort; p++ {
								
								if keyData, err = createPortVMapKeyData(pg.Proto, p); err != nil {
									logger.ErrorMessage(
										"packetFilterRouter.SetSecurityGroups(): Unable to create key data for port access rule in map '%s' for table '%s': %s",
										pgVmapName[i], chain.Table.Name, err.Error(),
									)
									continue
								}
								if err = r.nft.SetAddElements(sgPortVmap,
									[]nftables.SetElement{
										{
											Key:         keyData,
											VerdictData: &expr.Verdict{ Kind: verdict },
										},
									},
								); err != nil {
									logger.ErrorMessage(
										"packetFilterRouter.SetSecurityGroups(): Failed to add port access rule in map '%s' for table '%s': %s",
										pgVmapName[i], chain.Table.Name, err.Error(),
									)
								}
							}
						}
					}

				} else {
					rules = append(rules,
						&nftables.Rule{
							Table: chain.Table,
							Chain: chain,
							Position: insertPos[i],
							Exprs: append(sgExprs,
								// [ immediate reg 0 drop/accept ]
								&expr.Verdict{
									Kind: verdict,
								},
							),
						},
					)
				}
			}
		}		
		if err = r.saveFilterRules(sgKey, rules, insert); err != nil {
			logger.ErrorMessage(
				"packetFilterRouter.SetSecurityGroups(): Failed to create security group filter rule with sgKey '%s': %s",
				sgKey, err.Error(),
			)
			return err
		}	
	}
	return nil
}

func (r *packetFilterRouter) DeleteSecurityGroups(sgs []SecurityGroup, iifName string) error {

	var (
		err error
		ok  bool

		sgKey      string
		pgVmapName []string

		keyData    []byte
		sgPortVmap *nftables.Set
		elements   []nftables.SetElement

		vmapsTouched []*nftables.Set
	)

	for _, sg := range sgs {

		// get sg keys
		if sgKey, pgVmapName, err = sg.CreateSecurityGroupKeys(iifName); err != nil {
			logger.ErrorMessage("packetFilterRouter.DeleteSecurityGroups(): Error creating security group keys: %s", err.Error())
			continue
		}

		verdict := expr.VerdictAccept
		if sg.Deny {
			verdict = expr.VerdictDrop
		}

		if _, ok = r.ruleMap[sgKey]; ok {

			deleteFromTable:= []bool{ true, true }
			if sg.SrcNetwork.IsValid() || sg.DstNetwork.IsValid() {
				deleteFromTable[0] = sg.SrcNetwork.Addr().Is4() || sg.DstNetwork.Addr().Is4() // look for port group vmaps in ip4 table
				deleteFromTable[1] = sg.SrcNetwork.Addr().Is6() || sg.DstNetwork.Addr().Is6() // look for port group vmaps in ip6 table
			}

			for i, table := range r.table {
				if deleteFromTable[i] {

					if sgPortVmap, err = r.getSecurityGroupPortVMap(pgVmapName[i], nil); sgPortVmap != nil {						
						for _, pg := range sg.Ports {

							if pg.Proto != ICMP {
								for p := pg.FromPort; p <= pg.ToPort; p++ {
									if keyData, err = createPortVMapKeyData(pg.Proto, p); err != nil {
										logger.ErrorMessage(
											"packetFilterRouter.DeleteSecurityGroups(): Unable to create key data for port access rule in map '%s' for table '%s': %s",
											pgVmapName[i], table.Name, err.Error(),
										)										
										continue
									}
									if err = r.nft.SetDeleteElements(sgPortVmap,
										[]nftables.SetElement{
											{
												Key:         keyData,
												VerdictData: &expr.Verdict{ Kind: verdict },
											},
										},
									); err != nil {
										logger.ErrorMessage(
											"packetFilterRouter.DeleteSecurityGroups(): Failed to delete port access rule in map '%s' for table '%s': %s",
											pgVmapName[i], table.Name, err.Error(),
										)
									}
								}
							}
						}
						vmapsTouched = append(vmapsTouched, sgPortVmap)						

					} else if err != nil {
						logger.ErrorMessage(
							"No security group port vmap set found for sgKey '%s' with name '%s' for table '%s': %s",
							sgKey, pgVmapName[i], table.Name, err.Error(),
						)
					}
				}
			}
			if err = r.DeleteFilter(sgKey); err != nil {
				logger.ErrorMessage(
					"packetFilterRouter.DeleteSecurityGroups(): Failed to delete security group with key '%s': %s",
					sgKey, err.Error(),
				)
				continue
			}

			// Delete any port group vmaps that were touched that have no elements
			flush := false
			for _, sgPortVmap = range vmapsTouched {
				if elements, err = r.nft.GetSetElements(sgPortVmap); err != nil {
					logger.ErrorMessage(
						"packetFilterRouter.DeleteSecurityGroups(): Failed to get list of remaining elements for port group vmap '%s': %s",
						sgPortVmap.Name, err.Error(),
					)
					continue
				}
				if len(elements) == 0 {
					r.nft.DelSet(sgPortVmap)
					delete(r.sgPortVmaps, sgPortVmap.Name)
					flush = true
				}
			}
			if flush {
				if err = r.nft.Flush(); err != nil {
					logger.ErrorMessage(
						"packetFilterRouter.DeleteSecurityGroups(): Failed to delete empty maps that were associated with security group key '%s': %s",
						sgKey, err.Error(),
					)
				}
			}

		} else {
			return fmt.Errorf(
				"no filter rules associated for the security group with key '%s': %+v",
				sgKey, sg,
			)
		}
	}

	return nil
}

func (r *packetFilterRouter) getSecurityGroupPortVMap(vmapName string, table *nftables.Table) (*nftables.Set, error) {

	var (
		err error
		ok  bool

		keyType    nftables.SetDatatype
		sgPortVmap *nftables.Set
	)

	// get security group port vmap
	if sgPortVmap, ok = r.sgPortVmaps[vmapName]; !ok && table != nil {

		if keyType, err = nftables.ConcatSetType(
			nftables.TypeInetProto,
			nftables.TypeInetService,
		); err != nil {
			logger.ErrorMessage(
				"Failed to create concat key type for set sgPortVmap with name '%s' for table '%s': %s",
				vmapName, table.Name, err.Error(),
			)
			return nil, err
		}
		sgPortVmap = &nftables.Set{
			Name:          vmapName,
			Table:         table,
			IsMap:         true,
			KeyType:       keyType,
			DataType:      nftables.TypeVerdict,
		}
		if err = r.nft.AddSet(sgPortVmap, nil); err != nil {
			logger.ErrorMessage(
				"Failed to add set sgPortVmap with name '%s' for table '%s': %s",
				vmapName, table.Name, err.Error(),
			)
			return nil, err
		}
		r.sgPortVmaps[vmapName] = sgPortVmap
	}
	return sgPortVmap, nil
}

func (r *packetFilterRouter) ForwardPort(dstPort int, forwardPort int, forwardIP netip.Addr, proto Protocol) (string, error) {
	return r.ForwardPortOnIP(dstPort, forwardPort, netip.Addr{}, forwardIP, proto)
}

func (r *packetFilterRouter) DeleteForwardPort(dstPort int, forwardPort int, forwardIP netip.Addr, proto Protocol) error {
	return r.DeleteForwardPortOnIP(dstPort, forwardPort, netip.Addr{}, forwardIP, proto)
}

func (r *packetFilterRouter) ForwardPortOnIP(dstPort, forwardPort int, dstIP, forwardIP netip.Addr, proto Protocol) (string, error) {

	var (
		err error

		protoData []byte

		table  *nftables.Table
		chains []*nftables.Chain
		rules  []*nftables.Rule
	)

	isDstIPValid := dstIP.IsValid()

	is4 := forwardIP.Is4()
	addrLen, _, destOffset, _ := ipHeaderOffsets(is4)

	if table, chains, err = r.getTable(is4); err != nil {
		return "", err
	}

	switch proto {
	case ICMP:
		protoData = []byte{unix.IPPROTO_ICMP}
	case TCP:
		protoData = []byte{unix.IPPROTO_TCP}
	case UDP:
		protoData = []byte{unix.IPPROTO_UDP}
	default:
		return "", fmt.Errorf("unsupported protocol")
	}

	// add pre-routing dnat rule to forward to given ip:port
	rule := &nftables.Rule{
		Table: table,
		Chain: chains[natPreRoute],
	}
	// [ ip daddr <dstIP> ] tcp dport <dstPort> dnat to <forwardIP>:<forwardPort>
	if isDstIPValid {
		rule.Exprs = []expr.Any{
			// [ payload load 4b @ network header + 16 (dest addr) => reg 1 ]
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       destOffset,
				Len:          addrLen,
			},
			// cmp eq reg 1 <dstIP>
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dstIP.AsSlice(),
			},
		}
	}
	rule.Exprs = append(rule.Exprs,
		[]expr.Any{
			// [ meta load l4proto => reg 1 ]
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			// [ cmp eq reg 1 <protoData> ]
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     protoData,
			},
			// [ payload load 2b @ transport header + 2 => reg 1 ]
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       2, // Destination Port
				Len:          2,
			},
			// [ cmp eq reg 1 <dstPort> ]
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.BigEndian.PutUint16(uint16(dstPort)),
			},
			// [ immediate reg 1 <forwardIP> ]
			&expr.Immediate{
				Register: 1,
				Data:     forwardIP.AsSlice(),
			},
			// [ immediate reg 2 <forwardPort> ]
			&expr.Immediate{
				Register: 2,
				Data:     binaryutil.BigEndian.PutUint16(uint16(forwardPort)),
			},
			// [ nat dnat ip addr_min reg 1 addr_max reg 0 proto_min reg 2 proto_max reg 0 ]
			&expr.NAT{
				Type:        expr.NATTypeDestNAT,
				Family:      ipFamily(is4),
				RegAddrMin:  1,
				RegAddrMax:  1,
				RegProtoMin: 2,
				RegProtoMax: 2,
			},
		}...,
	)
	rules = append(rules, rule)

	// dest ip check expr
	ipDaddrExpr := []expr.Any{
		// [ payload load 4b @ network header + 16 (dest addr) => reg 1 ]
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       destOffset,
			Len:          addrLen,
		},
		// cmp eq reg 1 <forwardIP>
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     forwardIP.AsSlice(),
		},
	}
	// add post-routing masq rule
	rules = append(rules,
		&nftables.Rule{
			Table: table,
			Chain: chains[natPostRoute],
			// ip daddr <forwardIP> masquerade
			Exprs: append(
				ipDaddrExpr,
				// masq
				&expr.Masq{},
			),
		},
	)

	// add forward ip accept rule
	rules = append(rules,
		&nftables.Rule{
			Table: table,
			Chain: chains[filterForward],
			// ip daddr <forwardIP> accept
			Exprs: append(
				ipDaddrExpr,
				// accept
				&expr.Verdict{
					// [ immediate reg 0 accept ]
					Kind: expr.VerdictAccept,
				},
			),
		},
	)

	ruleKey := fmt.Sprintf("%s:%d>%s:%d|%s",
		dstIP.String(), dstPort,
		forwardIP.String(), forwardPort,
		string(proto),
	)
	return ruleKey, r.saveFilterRules(ruleKey, rules, false)
}

func (r *packetFilterRouter) DeleteForwardPortOnIP(dstPort, forwardPort int, dstIP, forwardIP netip.Addr, proto Protocol) error {
	return r.DeleteFilter(
		fmt.Sprintf("%s:%d>%s:%d|%s",
			dstIP.String(), dstPort,
			forwardIP.String(), forwardPort,
			string(proto),
		),
	)
}

func (r *packetFilterRouter) ForwardTraffic(srcItfName, dstItfName string, srcNetwork, dstNetwork netip.Prefix, withNat bool) (string, error) {

	var (
		err error

		table  *nftables.Table
		chains []*nftables.Chain
		rules  []*nftables.Rule
	)

	if srcNetwork.Addr().BitLen() != dstNetwork.Addr().BitLen() {
		return "", fmt.Errorf("attempt to create forwarding rules between incompatible network address spaces")
	}

	is4 := srcNetwork.Addr().Is4()
	addrLen, srcOffset, destOffset, _ := ipHeaderOffsets(is4)

	if table, chains, err = r.getTable(is4); err != nil {
		return "", err
	}

	iifname := []byte(srcItfName+"\x00")
	oifname := []byte(dstItfName+"\x00")

	// ip saddr <srcNetwork> ip daddr <dstNetwork>
	srcNetworkMask := net.CIDRMask(srcNetwork.Bits(), int(addrLen)*8)
	ipSrcDstExprs := []expr.Any{
		// [ payload load 4b @ network header + 12 (src addr) => reg 1 ]
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       srcOffset,
			Len:          addrLen,
		},
		// [ bitwise reg 1 = (reg=1 & <srcNetwork Mask> ) ^ 0x00000000 ]
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            addrLen,
			Mask:           []byte(srcNetworkMask),
			Xor:            make([]byte, addrLen),
		},
		// [ cmp eq reg 1 <srcNetwork in canonical form> ]
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     srcNetwork.Masked().Addr().AsSlice(),
		},
	}
	if dstNetwork != prefixWorld4 && dstNetwork != prefixWorld6 {
		dstNetworkMask := net.CIDRMask(dstNetwork.Bits(), int(addrLen)*8)
		ipSrcDstExprs = append(ipSrcDstExprs,
			// [ payload load 4b @ network header + 16 (dest addr) => reg 1 ]
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       destOffset,
				Len:          addrLen,
			},
			// [ bitwise reg 1 = (reg=1 & dstNetwork Mask> ) ^ 0x00000000 ]
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            addrLen,
				Mask:           []byte(dstNetworkMask),
				Xor:            make([]byte, addrLen),
			},
			// [ cmp eq reg 1 <dstNetwork in canonical form> ]
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dstNetwork.Masked().Addr().AsSlice(),
			},
		)
	}

	rules = append(rules,
		&nftables.Rule{
			Table: table,
			Chain: chains[filterForward],
			Exprs: append(
				// iifname "srcItf" oifname "i" ip saddr <srcNetwork>ip daddr <dstNetwork> accept
				[]expr.Any{
					// [ meta load iifname => reg 1 ]
					&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
					// [ cmp eq reg 1 <iifname> ]
					&expr.Cmp{
						Op:       expr.CmpOpEq,
						Register: 1,
						Data:     iifname,
					},
					// [ meta load oifname => reg 1 ]
					&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
					// [ cmp eq reg 1 <oifname> ]
					&expr.Cmp{
						Op:       expr.CmpOpEq,
						Register: 1,
						Data:     oifname,
					},
				},
				append(
					ipSrcDstExprs,
					//[ immediate reg 0 accept ]
					&expr.Verdict{
						Kind: expr.VerdictAccept,
					},
				)...
			),
		},
	)

	if withNat {
		rules = append(rules,
			&nftables.Rule{
				Table: table,
				Chain: chains[natPostRoute],
				Exprs: append(
					// oifname "i" ip saddr <srcNetwork> ip daddr <dstNetwork> masquerade
					[]expr.Any{
						// meta load oifname => reg 1
						&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
						// [ cmp eq reg 1 <oifname> ]
						&expr.Cmp{
							Op:       expr.CmpOpEq,
							Register: 1,
							Data:     oifname,
						},
					},
					append(
						ipSrcDstExprs,
						// masq
						&expr.Masq{},
					)...
				),
			},
		)
	}

	ruleKey := fmt.Sprintf("%s:%s>%s:%s",
		srcItfName, srcNetwork.String(),
		dstItfName, dstNetwork.String(),
	)
	return ruleKey, r.saveFilterRules(ruleKey, rules, false)
}

func (r *packetFilterRouter) DeleteForwardTraffic(srcItfName, dstItfName string, srcNetwork, dstNetwork netip.Prefix) error {
	return r.DeleteFilter(
		fmt.Sprintf("%s:%s>%s:%s",
			srcItfName, srcNetwork.String(),
			dstItfName, dstNetwork.String(),
		),
	)
}

func (r *packetFilterRouter) saveFilterRules(key string, rules []*nftables.Rule, insert bool) error {

	var (
		err error

		savedRules []*nftables.Rule
		foundRule  *nftables.Rule
	)

	// add rules that do not already exist
	for _, rule := range rules {
		if savedRules, err = r.nft.GetRules(rule.Table, rule.Chain); err != nil {
			return err
		}
		if foundRule = findRuleInList(rule, savedRules); foundRule == nil {
			if insert {
				r.nft.InsertRule(rule)
			} else {
				r.nft.AddRule(rule)
			}
		}
	}
	// flush rules
	if err = r.nft.Flush(); err != nil {
		return err
	}
	// get rule handles
	if len(rules) > 0 {
		for _, rule := range rules {
			if savedRules, err = r.nft.GetRules(rule.Table, rule.Chain); err != nil {
				return err
			}
			if foundRule = findRuleInList(rule, savedRules); foundRule == nil {
				return fmt.Errorf("a saved rule instance was not found for rule with key '%s': %+v", key, rule)
			}
			rule.Handle = foundRule.Handle

			ruleRefKey := fmt.Sprintf("%s_%d", rule.Table.Name, rule.Handle)
			r.ruleRef[ruleRefKey] = r.ruleRef[ruleRefKey]+1
		}
		r.ruleMap[key] = append(r.ruleMap[key], rules...)
	}
	return nil
}

func (r *packetFilterRouter) DeleteFilter(key string) error {

	var (
		err   error
		ok    bool
		rules []*nftables.Rule
	)

	if rules, ok = r.ruleMap[key]; ok {
		for _, rule := range rules {
			// delete rule only when its ref count
			// is zero. this ensures any shared rules
			// are not deleted
			ruleRefKey := fmt.Sprintf("%s_%d", rule.Table.Name, rule.Handle)
			ruleRefCount := r.ruleRef[ruleRefKey]
			if ruleRefCount == 1 {
				if err = r.nft.DelRule(rule); err != nil {
					return err
				}	
				delete(r.ruleRef, ruleRefKey)

			} else {
				r.ruleRef[ruleRefKey] = r.ruleRef[ruleRefKey]-1
			}
		}
		delete(r.ruleMap, key)

	} else {
		return fmt.Errorf(
			"no filter rules associated with key '%s' was found",
			key,
		)
	}

	return r.nft.Flush()
}

func (r *packetFilterRouter) Clear() {

	if r.table != nil {
		for _, t := range r.table {
			r.nft.DelTable(t)
		}
		if err := r.nft.Flush(); err != nil {
			logger.ErrorMessage(
				"packetFilterRouter.reset(): Error commiting deletion of mycs nftables: %s",
				err.Error(),
			)
		}

		r.table = nil
		r.chains = nil
	}
}

func (r *packetFilterRouter) String() string {

	var (
		out bytes.Buffer
	)

	fmt.Fprintln(&out, "Packet Filter State\n===================")
	fmt.Fprintf(&out, "\nSaved Rule Map:\n\n")
	for key, rules := range r.ruleMap {
		fmt.Fprintf(&out, "Rule Key: %s\n", key)
		for _, rule := range rules {
			fmt.Fprintf(&out, "..Rule: [ table: %s, chain: %s, handle: %d ]\n", rule.Table.Name, rule.Chain.Name, rule.Handle)
			for i, e := range rule.Exprs {
				fmt.Fprintf(&out, "....expr:[%d]: %t\n", i, e)
			}
		}
	}
	fmt.Fprintf(&out, "\nSaved handles\n")
	for handle, refCount := range r.ruleRef { 
		fmt.Fprintf(&out, "- %s : %d ", handle, refCount)
		for key, rules := range r.ruleMap { 
			for _, rule := range rules  {
				if handle == fmt.Sprintf("%s_%d", rule.Table.Name, rule.Handle) { 
					fmt.Fprintf(&out, " (%s)", key) 
				}
			}
		}
		fmt.Fprintln(&out)
	}
	return out.String()
}

// security group functions

func (sg SecurityGroup) CreateSecurityGroupKeys(iifName string) (string, []string, error) {

	var (
		err error

		sgKeyB            bytes.Buffer
		sgKey, pgVmapName string
	)

	if len(iifName) > 0 {
		sgKeyB.WriteByte('_')
		sgKeyB.WriteString(iifName)
	}
	if sg.SrcNetwork.IsValid() {
		sgKeyB.WriteByte('_')
		sgKeyB.WriteString(sg.SrcNetwork.String())
	}
	if sg.DstNetwork.IsValid() {
		sgKeyB.WriteByte('_')
		sgKeyB.WriteString(sg.DstNetwork.String())
	}

	if pgVmapName, err = utils.HashString(sgKeyB.String(), "sgs"); err != nil {
		return "", nil,
			fmt.Errorf(
				"failed to create port group vmap name from sgKey signature prefix '%s': %s", 
				sgKeyB.String(), err.Error(),
			)
	}
	for _, pg := range sg.Ports {
		sgKeyB.WriteByte('-')
		sgKeyB.Write([]byte(pg.Proto))
		if pg.Proto != ICMP {
				// add to sgKey signature
				fmt.Fprintf(&sgKeyB, ".%d-%d", pg.FromPort, pg.ToPort)
		}
	}
	if sgKey, err = utils.HashString(sgKeyB.String(), "sgs"); err != nil {
		return "", nil,
			fmt.Errorf(
				"failed to create to sgKey from sgKey signature '%s': %s", 
				sgKeyB.String(), err.Error(),
			)
	}
	return sgKey, 
		[]string{ pgVmapName+"_0", pgVmapName+"_1" },
		nil
}

// helper functions

// returns ip addrLen and srcOffset, destOffset in ip header and protoOffset in transport header
func ipHeaderOffsets(is4 bool) (addrLen uint32, srcOffset uint32, destOffset uint32, protoOffset uint32) {
	if (is4) {
		return 4, 12, 16, 9
	} else {
		return 16, 8, 24, 6
	}
}

// returns the ip family
func ipFamily(is4 bool) uint32 {
	if is4 {
		return unix.NFPROTO_IPV4
	} else {
		return unix.NFPROTO_IPV6
	}
}

// adds a string to a byte array and reverses it
// this addresses instances where string values
// need to be passed to netlink in a reverse
// byte array of fixed length (this could be a
// bug in nftables lib)
func byteString(s string, l int) []byte {

	if len(s) > l {
		panic("byteString: attempting to create a string larger than the given length")
	}

	b := make([]byte, l)
	copy(b, []byte(s))
	return b
}

// asynchronously match given rule
// with that of the rules in list
func findRuleInList(rule *nftables.Rule, ruleList []*nftables.Rule) *nftables.Rule {

	var (
		err error

		ruleExprData [][]byte
		exprData     []byte

		foundRule *nftables.Rule
	)

	// marshal given rule's exprs for matching

	for _, e := range rule.Exprs {
		if exprData, err = expr.Marshal(byte(rule.Table.Family), e); err != nil {
			logger.ErrorMessage("findRuleInList(): Rule marshal error: %s", err.Error())
			return nil
		}
		ruleExprData = append(ruleExprData, exprData)
	}

	// fmt.Println(debugPrintRule("Looking up rule in list", rule))

	foundRuleC := make(chan *nftables.Rule, len(ruleList))
	checkRuleMatch := func(rule, matchRule *nftables.Rule, ruleExprData [][]byte) {

		// fmt.Println(debugPrintRule("Check match of rule from list", matchRule))

		if matchRule.Flags == rule.Flags &&
			bytes.Equal(matchRule.UserData, rule.UserData) {

			for i, e := range matchRule.Exprs {
				if exprData, err = expr.Marshal(byte(matchRule.Table.Family), e); err != nil {
					logger.ErrorMessage("findRuleInList(): Rule marshal error: %s", err.Error())
					foundRuleC <-nil
					return
				}
				if !bytes.Equal(exprData, ruleExprData[i]) {
					foundRuleC <-nil
					return
				}
			}
			foundRuleC <-matchRule
			return
		}
		foundRuleC <-nil
	}
	for _, matchRule := range ruleList {
		go checkRuleMatch(rule, matchRule, ruleExprData)
	}
	for i := 0; i < len(ruleList); i++ {
		if foundRule = <-foundRuleC; foundRule != nil {
			break
		}
	}
	// fmt.Printf("Found rule: %+v\n", foundRule)
	return foundRule
}

// create concatenated key data for port group vmap
func createPortVMapKeyData(proto Protocol, port int) ([]byte, error) {
	// key fields should have 4 boundaries
	//
	// byte[0..3] => protocol
	// byte[4..7] => port
	//
	keyData := make([]byte, 8)
	// protocol
	switch proto {
	case ICMP:
		keyData[0] = unix.IPPROTO_ICMP
	case TCP:
		keyData[0] = unix.IPPROTO_TCP
	case UDP:
		keyData[0] = unix.IPPROTO_UDP
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", string(proto))
	}
	// destination port
	copy(keyData[4:], binaryutil.BigEndian.PutUint16(uint16(port)))
	return keyData, nil
}

// create separate lists of ip4 and ip6 set elements given a combine list of ip4/6 ips
func createIPSetElements(ips []netip.Addr) [][]nftables.SetElement {

	ipSetElems := make([][]nftables.SetElement, 2)
	for _, ip := range ips {
		if ip.Is4() {
			ipSetElems[0] = append(ipSetElems[0], nftables.SetElement{ Key: ip.AsSlice() })
		} else {
			ipSetElems[1] = append(ipSetElems[1], nftables.SetElement{ Key: ip.AsSlice() })
		}
	}
	return ipSetElems
}

// prints rule for debugging
func debugPrintRule(msg string, rule *nftables.Rule) string {

	var (
		out bytes.Buffer
	)

	fmt.Fprintf(&out, "BEGIN=> %s\n", msg)
	fmt.Fprintf(&out, "  Rule....: %+v\n", rule)
	for i, e := range rule.Exprs {
		fmt.Fprintf(&out, "    expr..:[%d]: %t\n", i, e)
	}
	fmt.Fprintf(&out, "END=> %s", msg)
	return out.String()
}
