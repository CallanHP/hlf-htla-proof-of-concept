/*
 * Sample hash timelock contract for Hyperledger fabric. This variant uses an
 * abstract concept of a 'proposal', which can be placed into a pending state,
 * then is only confirmed through production of the pre-image.
 *
 * This code leaves a lot of sections empty, to be filled with appropriate
 * logic for whatever the concrete use case is.
 */

package main

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/peer"
)

//HashTimeLockContract struct to define the smart contract object
type HashTimeLockContract struct {
}

//Init method for handling instantiation/upgrade
func (s *HashTimeLockContract) Init(stub shim.ChaincodeStubInterface) peer.Response {
	return shim.Success(nil)
}

//Invoke method for handling chaincode operations
func (s *HashTimeLockContract) Invoke(stub shim.ChaincodeStubInterface) peer.Response {
	// Retrieve the requested Smart Contract function and arguments
	function, args := stub.GetFunctionAndParameters()
	// Route to the appropriate handler function to interact with the ledger appropriately
	switch function {
	case "createProposal":
		return s.createProposal(stub, args)
	case "confirmProposal":
		return s.confirmProposal(stub, args)
	case "invalidateProposal":
		return s.invalidateProposal(stub, args)
	default:
		return shim.Error("Invalid Smart Contract function name.")
	}
}

/*
 * Entry point for the operation - takes a proposal and a hash, then creates the
 * entry tagged as PENDING. Fires events which can be consumed by external
 * clients to handle things like automated invalidation (timelocking), alerting
 * of the intended middle-man consumer, etc.
 *
 * In this example, we support SHA256, SHA384 and SHA512, and expect the hash
 * to be provided as a hexadecimal string
 *
 * Returns a proposal id - which here is taken from the proposal object, but
 * could be generated, etc...
 */
func (s *HashTimeLockContract) createProposal(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	var err error
	//Validate the args, expect 3, the proposal, the hash and the hashing algorithm
	if len(args) != 3 {
		return shim.Error("Invalid arguments to createProposal, expected proposal, hash, hashingAlg.")
	}
	//Check if it is a valid hashing algorithm
	validAlg := false
	for _, a := range validHashingAlgorithms {
		if a == args[2] {
			validAlg = true
		}
	}
	if validAlg == false {
		return shim.Error("Only these hashing algorithms are supported: " + strings.Join(validHashingAlgorithms, ", "))
	}
	proposal := proposalEntry{Proposal: abstractProposal{}, Status: PendingStatus, Hash: args[1], HashAlgorithm: args[2]}
	err = json.Unmarshal([]byte(args[0]), &proposal.Proposal)
	if err != nil {
		return shim.Error("Error parsing provided proposal definition - " + err.Error())
	}
	if proposal.Proposal.ProposalID == "" {
		return shim.Error("No proposalId provided as part of proposal.")
	}
	if proposal.Proposal.Handler == "" {
		//There should probably be a lot more validation of this handler - but we
		//will just accept what is passed for this sample
		return shim.Error("No proposalHandler provided as part of proposal.")
	}
	/*
	 * All of your awesome validation logic goes here - maybe we need access control,
	 * maybe we need to validate the proposal handler is appropriate?
	 */

	//Write the proposal to state
	proposalAsBytes, err := json.Marshal(proposal)
	if err != nil {
		return shim.Error("Error building proposal definition - " + err.Error())
	}
	err = stub.PutState(proposalPrefix+proposal.Proposal.ProposalID, proposalAsBytes)
	if err != nil {
		return shim.Error("Error writing proposal to state - " + err.Error())
	}
	//Fire appropriate events
	proposalCreatedEvent := ProposalCreatedEventObject{ProposalID: proposal.Proposal.ProposalID}
	proposalEventAsBytes, err := json.Marshal(proposalCreatedEvent)
	if err != nil {
		return shim.Error("Error building proposal event definition - " + err.Error())
	}
	//Event for the provided handler
	err = stub.SetEvent(proposal.Proposal.Handler+ProposalCreatedHandlerEvent, proposalEventAsBytes)
	//Event for the timeout client
	err = stub.SetEvent(ProposalCreateTimeoutEvent, proposalEventAsBytes)
	return shim.Success(nil)
}

/*
 * Invoked to transition a proposal from PENDING to CONFIRMED - takes a
 * proposalId and a pre-image that corresponds with the initially provided
 * hash. Fires an event on this state transition, which is indended to
 * allow the middle-man to obtain the pre-image, then use that to confirm
 * the transaction in the other channel.
 * In most practical implementations, there would be some business specific
 * operations which would be performed due to this transition, but as this
 * sample is getting away with using the same contract in both places, it
 * doesn't do that here.
 */
func (s *HashTimeLockContract) confirmProposal(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	var err error
	//Validate the args, expect 2, the proposalId and the pre-image of the hash
	//for that proposalId
	if len(args) != 2 {
		return shim.Error("Invalid arguments to confirmProposal, expected proposalId, pre-image.")
	}
	//Retreive the proposal referenced
	proposalAsBytes, err := stub.GetState(proposalPrefix + args[0])
	if err != nil {
		return shim.Error("Error while retreiving the stored proposal from state - " + err.Error())
	}
	if proposalAsBytes == nil {
		return shim.Error("No such proposal. It may have expired and been invalidated.")
	}
	proposal := proposalEntry{}
	err = json.Unmarshal(proposalAsBytes, &proposal)
	if err != nil {
		return shim.Error("Error while parsing the proposal stored in state - " + err.Error())
	}

	/*
	 * All of your awesome validation logic goes here - maybe we need to
	 * validate the transaction creator matches the tagged handler?
	 * Maybe we wrote time-bound conditions into the proposal, and can't
	 * confirm it after the end of that window, even if the hash is valid?
	 * (even though trusting timestamps in HLF is hard...)
	 */

	//Validate whether the supplied pre-image is valid for this proposal
	//Going to compare hexadecimal strings
	var hasher hash.Hash
	switch proposal.HashAlgorithm {
	case "SHA256":
		hasher = sha256.New()
		break
	case "SHA384":
		hasher = sha512.New384()
		break
	case "SHA512":
		hasher = sha512.New()
		break
	default:
		return shim.Error("The hash algorithm which was recorded in the proposal is not supported.")
	}
	hasher.Write([]byte(args[1]))
	if hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(proposal.Hash) {
		return shim.Error("Invalid Pre-image supplied.")
	}
	//Mark the proposal as confirmed
	proposal.Status = ConfirmStatus
	proposalAsBytes, err = json.Marshal(proposal)
	if err != nil {
		return shim.Error("Error when marshaling proposal - " + err.Error())
	}
	err = stub.PutState(proposalPrefix+args[0], proposalAsBytes)
	//Fire an event to inform middle actor to allow replaying into other channel
	proposalConfirmedEvent := ProposalConfirmedEventObject{ProposalID: args[0], PreImage: args[1]}
	proposalEventAsBytes, err := json.Marshal(proposalConfirmedEvent)
	if err != nil {
		return shim.Error("Error building proposal event definition - " + err.Error())
	}
	err = stub.SetEvent(ProposalConfirmedHandlerEvent, proposalEventAsBytes)
	return shim.Success(nil)
}

/*
 * Function that can be used to invalidate a proposal in PENDING state.
 * This is intended to facilitate the timelocking - where if a proposal hasn't
 * been confirmed, it gets deleted.
 * Fails if invoked on a CONFIRMED proposal.
 */
func (s *HashTimeLockContract) invalidateProposal(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	var err error
	//Validate the args, expect 1, the proposalId
	if len(args) != 1 {
		return shim.Error("Invalid arguments to invalidateProposal, expected proposalId")
	}
	/*
	 * All sorts of validation logic about who can invalidate a proposal, maybe
	 * check whether we have exceeded a minimum elapsed time or something?
	 */
	proposalBytes, err := stub.GetState(proposalPrefix + args[0])
	if err != nil {
		return shim.Error("Error retreiving stored proposal from state")
	}
	proposal := proposalEntry{}
	err = json.Unmarshal(proposalBytes, &proposal)
	if err != nil {
		return shim.Error("Error while parsing the proposal stored in state - " + err.Error())
	}
	if proposal.Status != PendingStatus {
		return shim.Error("Only pending proposals can be timed out.")
	}
	//Delete the proposal
	err = stub.DelState(proposalPrefix + args[0])
	if err != nil {
		return shim.Error("Error while deleting proposal from state.")
	}
	return shim.Success(nil)
}

func main() {
	// Create a new Smart Contract
	err := shim.Start(new(HashTimeLockContract))
	if err != nil {
		fmt.Printf("Error creating new Smart Contract: %s", err)
	}
}
