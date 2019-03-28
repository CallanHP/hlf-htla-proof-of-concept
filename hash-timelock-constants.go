package main

//Prefixes for keys

const proposalPrefix string = "_proposal_"

//Constants for internally set values

//PendingStatus is the default state in which new proposals are placed
const PendingStatus = "PENDING"

//ConfirmStatus is used after the preimage is supplied
const ConfirmStatus = "CONFIRMED"

//Object representations

//abstractProposal is a placeholder for a real proposal struct
type abstractProposal struct {
	ProposalID string `json:"proposalId"`
	Handler    string `json:"proposalHandler"`
}

//proposalEntry represents the object which is stored in the state,
//this could be handled with composite keys if preferred, which would
//be better in some scenarios
type proposalEntry struct {
	Proposal      abstractProposal `json:"proposal"`
	Status        string           `json:"status"`
	Hash          string           `json:"hash"`
	HashAlgorithm string           `json:"hashAlgorithm"`
}

//Valid hashing algorithms
var validHashingAlgorithms = []string{"SHA256", "SHA384", "SHA512"}

//Events

//ProposalCreatedHandlerEvent is fired when an initial proposal is added, it is
//intended to be prefixed by the intended handler organisation
const ProposalCreatedHandlerEvent = "_PROPOSAL_CREATED"

//ProposalCreateTimeoutEvent is fired when an initial proposal is added, it is
//intended to be handled by a client which makes an invalidate call after some
//configured client side timeout window.
const ProposalCreateTimeoutEvent = "PROPOSAL_CREATED"

//ProposalCreatedEventObject provides a structure to simplify the creation of a
//serialised object with the event details. At present, it has nothing in it,
//but obviously could be augmented with relevant additonal details
type ProposalCreatedEventObject struct {
	ProposalID string `json:"proposalId"`
}

//ProposalConfirmedHandlerEvent is fired when a proposal is confirmed, this
//could be prefixed with the proposed handler or similar, but we are not
//capturing it here.
const ProposalConfirmedHandlerEvent = "PROPOSAL_CONFIRMED"

//ProposalConfirmedEventObject provides a structure to simplify the creation of a
//serialised object with the event details. At present, it has nothing in it,
//but obviously could be augmented with relevant additonal details
type ProposalConfirmedEventObject struct {
	ProposalID string `json:"proposalId"`
	PreImage   string `json:"preImage"`
}
