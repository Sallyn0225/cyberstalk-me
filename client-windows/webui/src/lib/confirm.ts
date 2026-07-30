/**
 * The confirmation gate in front of expose_title.
 *
 * expose_title makes a process report its raw window title verbatim, publicly.
 * It is the one place in this product where the sanitization can be switched
 * off, so switching it on cannot be a single click.
 *
 * The phrase to type is the process name itself, not a fixed sentence. A fixed
 * sentence becomes muscle memory after the second time; the process name forces
 * the user to look at which application they are about to expose.
 */

/** confirmationPhrase is what the user has to type to expose a process. */
export function confirmationPhrase(process: string): string {
  return process.trim()
}

/**
 * confirmationMatches compares what was typed against the required phrase.
 *
 * Case and surrounding whitespace are forgiven: the point is deliberate intent,
 * not transcription accuracy, and process names are matched case-insensitively
 * everywhere else in this product too.
 */
export function confirmationMatches(input: string, process: string): boolean {
  const required = confirmationPhrase(process)
  if (!required) return false
  return input.trim().toLowerCase() === required.toLowerCase()
}
