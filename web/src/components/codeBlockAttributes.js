// Shared TipTap attributes for metadata that lives on the surrounding <pre>.
// Both editors must retain these attributes when parsing and serializing HTML.
export function codeBlockMetadataAttributes() {
  return {
    wrapped: {
      default: false,
      parseHTML: element => element.getAttribute('data-wrapped') === 'true',
      renderHTML: attributes => (attributes.wrapped ? { 'data-wrapped': 'true' } : {}),
    },
    infoString: {
      default: null,
      parseHTML: element => element.getAttribute('data-info-string'),
      renderHTML: attributes => (
        attributes.infoString ? { 'data-info-string': attributes.infoString } : {}
      ),
    },
  }
}
