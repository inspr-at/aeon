// SPDX-License-Identifier: AGPL-3.0-only
import { isNavigationFailure, NavigationFailureType, type Router } from 'vue-router'

// Register before starting a navigation: its successor may finish before the
// cancelled navigation's promise settles. Ignore intermediate cancellations.
export function settledNavigation(router: Router) {
  let stop = () => {}
  const promise = new Promise<void>(resolve => {
    stop = router.afterEach((_to, _from, failure) => {
      if (isNavigationFailure(failure, NavigationFailureType.cancelled)) return
      stop()
      resolve()
    })
  })
  return { promise, stop }
}
