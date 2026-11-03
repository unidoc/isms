// Which invite button is the form's submit button. Exactly one is, so Enter in
// a field never does the other action: with email set up Enter sends the
// email; without it Enter creates the link. The other button is type="button".
export function inviteButtonTypes(emailEnabled) {
  return emailEnabled
    ? { email: 'submit', link: 'button' }
    : { email: 'button', link: 'submit' }
}
