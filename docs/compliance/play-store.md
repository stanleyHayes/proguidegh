# Google Play — submission compliance

Covers `apps/tourist-mobile` (`gh.proguide.tourist`) and `apps/guide-mobile`
(`gh.proguide.guide`). Data values come from
[`data-inventory.md`](./data-inventory.md).

## Policies that actually bite this app

| Policy | Requirement | Status |
|---|---|---|
| **Data deletion** | An in-app deletion path **and** a web URL reachable without installing the app. | ✅ In-app `/privacy`; web `https://proguidegh.com/account/delete` (`apps/{tourist,guide}-web/app/account/delete`). |
| **Data safety** | Declare every collected type, why, and whether it is shared. | ✅ Answers below, derived from the data inventory. |
| **Background location** | Prominent in-app disclosure before the prompt, a listing declaration, **and a demo video**. | ✅ Disclosure + foreground service. ⛔ Demo video is M-18 (Human). |
| **Foreground service types** | Android 14+ requires a declared `foregroundServiceType`. | ✅ `android:foregroundServiceType="location"` ships in `expo-location`'s own manifest; `FOREGROUND_SERVICE_LOCATION` is granted by its config plugin. |
| **Payments** | Play Billing is required for **digital** goods only. Tours are real-world services. | ✅ Paystack hosted page; no in-app card capture. |
| **Target API level** | Must meet the current Play requirement. | ✅ Inherited from Expo SDK 57 / React Native 0.86; re-check at submission — the threshold moves annually. |
| **Families / ads** | Not a children's app; no ads. | ✅ Not applicable. |

## Data safety form answers

Same source table as Apple; the two forms must not disagree.

**Both apps**
- Personal info › Name, Email address, Phone number — *collected*, **not shared**, required, purpose: App functionality / Account management.
- Financial info › Purchase history — *collected*, not shared, purpose: App functionality.
- App activity › Other actions (audit trail) — *collected*, not shared.
- Encrypted in transit: **Yes**. Users can request deletion: **Yes**.

**Guide app additionally**
- Location › Precise location — *collected*, **shared** with the tourist on the
  active booking, purpose: App functionality (dispatch, live tracking, safety).
- Personal info › Other info (verification documents) — collected, not shared.
- Financial info › Payment info (tokenised payout reference) — collected, not shared.

**Neither app**
- No advertising ID, no data collected for advertising or analytics-based
  tracking, no data sold.

## Background location declaration

Play requires a written justification plus a video. Justification text:

> ProGuideGH Guide is used by certified tour guides in Ghana. Background
> location is required so that a tourist who has booked a guide can see that
> guide approaching the meeting point, and so our operations team can locate a
> guide who raises an emergency SOS during a tour. Location is collected only
> while the guide is online or on an active tour, is visible only to the tourist
> on that booking and to operations staff, and stops as soon as the tour
> completes or the guide goes offline. An in-app screen explains this before any
> permission is requested.

The **video** (M-18, Human) must show: the disclosure screen → the OS prompt →
the feature working with the app backgrounded. Reviewers reject text-only
justifications for background location.

## Account deletion URL

Submit `https://proguidegh.com/account/delete`.

It must work for someone who never installed the app. The page handles the
signed-out case explicitly: it explains that identity is verified before
deletion, links to sign-in, and gives a `privacy@proguidegh.com` fallback for
users who have lost access to their email. A URL that only 401s will be
rejected.

## Before submitting

- [ ] EXT-3: Google Play Console account active
- [ ] `npx eas init` has written `extra.eas.projectId` + `owner` to both `app.json`s
- [ ] Data safety form completed for both apps from the table above
- [ ] Background-location video recorded and uploaded (guide app)
- [ ] Deletion URL live and reachable while signed out
- [ ] Privacy policy URL returns 200 (currently a placeholder — P9-06)
- [ ] Target API level meets the current Play threshold
- [ ] `npx eas build --profile production --platform android` produces an app bundle
- [ ] Internal testing track validated on a physical device
