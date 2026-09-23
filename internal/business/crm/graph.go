// SPDX-License-Identifier: AGPL-3.0-only

package crm

// Node kind slugs the CRM graph understands. Project and quote are R1/R4
// kinds this package does not seed.
const (
	Organisation = "organisation"
	Contact      = "contact"
	Project      = "project"
	Quote        = "quote"
)

// Relation types stored on node_relations. Direction is the stored source
// and target; these types are never sorted the way "relates" is.
const (
	CustomerOf = "customer_of"
	ContactFor = "contact_for"
)

// GraphType reports whether relType is a CRM link the relations handler must
// check for kind and direction.
func GraphType(relType string) bool {
	return relType == CustomerOf || relType == ContactFor
}

// GraphAllowed reports whether a live source kind may point at a live target
// kind. customer_of is organisation to project or quote. contact_for is
// contact to organisation, project, or quote. Any other type is refused.
func GraphAllowed(sourceKind, targetKind, relType string) bool {
	switch relType {
	case CustomerOf:
		return sourceKind == Organisation && (targetKind == Project || targetKind == Quote)
	case ContactFor:
		return sourceKind == Contact && (targetKind == Organisation || targetKind == Project || targetKind == Quote)
	default:
		return false
	}
}
