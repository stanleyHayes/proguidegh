# Apple App Store — submission compliance

Covers `apps/tourist-mobile` (`gh.proguide.tourist`) and `apps/guide-mobile`
(`gh.proguide.guide`). Data values come from
[`data-inventory.md`](./data-inventory.md) — fill the App Store Connect forms
from that table, not from memory.

## Guidelines that actually bite this app

| Guideline | Requirement | Status |
|---|---|---|
| **5.1.1(v)** Account deletion | An app that lets users create an account must let them delete it **from inside the app**. Deactivation is not deletion. | ✅ `/privacy` in both apps → `DELETE /api/v1/me`. Two-step confirm, states it is permanent, lists what is retained. |
| **5.1.1(i)** Privacy policy | Reachable link, in-app and on the listing. | ✅ `GET /api/v1/legal/policies` is public; `/privacy` links it. ⛔ The URLs 404 until Legal publishes the pages (P9-06). |
| **5.1.2** Data minimisation | Only collect what the feature needs. | ✅ Tourist app requests no location at all. |
| **5.1.5** Location services | Explain the use before requesting; only request what the feature needs. | ✅ Rationale screen precedes the OS prompt; guide app only. |
| **2.5.4** Background location | Background modes must be needed for the advertised feature. | ✅ `UIBackgroundModes: [location]`, used for live tour tracking and emergency response. |
| **3.1.1** In-app purchase | IAP is required for **digital** goods only. Tours are real-world services, so external payment is permitted. | ✅ Paystack hosted page via `expo-web-browser`; no card fields in-app. |
| **Privacy manifest** | `PrivacyInfo.xcprivacy` with required-reason APIs and collected data types. | ✅ `ios.privacyManifests` in each `app.json`. |
| **ATT** | Required only if tracking across apps/sites. | ✅ Not applicable — `NSPrivacyTracking: false`, no ad SDKs. No ATT prompt. |
| **Sign in with Apple** | Required only if you offer third-party social login. | ✅ Not applicable — email/password only. Revisit if social login is ever added. |

## Info.plist usage strings

Set via the `expo-location` plugin in `apps/guide-mobile/app.json`. Apple
rejects vague strings ("we need your location"); these name who sees the data
and when.

- `NSLocationAlwaysAndWhenInUseUsageDescription` / `NSLocationWhenInUseUsageDescription`:
  > ProGuideGH shares your location with the tourist and our operations team only
  > while you are online or on an active tour, so you can be matched to nearby
  > jobs and reached in an emergency.

The tourist app declares no location strings, because it requests no location.

## App Privacy answers (App Store Connect)

Derive every answer from [`data-inventory.md`](./data-inventory.md).

- **Tourist app:** Email Address, Phone Number, Name, Purchase History,
  Customer Support, Crash Data. All *Linked to the user*, none *Used for
  tracking*, purpose *App Functionality*.
- **Guide app:** the above **plus Precise Location**, and Sensitive Info for
  verification documents.
- *Used to Track You*: **No** for every item, in both apps.

## Review notes to submit

Reviewers reject background location they cannot reproduce. Put this in the
review notes:

> ProGuideGH Guide is used by certified Ghanaian tour guides. Background
> location is collected only while a guide is on an active tour, so the tourist
> can see the guide approaching and so our operations team can respond to an SOS.
> Collection stops the moment the tour completes or the guide goes offline. The
> in-app screen "Location sharing" explains this before any permission prompt.
>
> Payment uses the Paystack hosted checkout page for a real-world service (a
> guided tour); no digital goods are sold.
>
> Account deletion: sign in → Privacy & data → Delete my account.

Provide a demo guide account **with a booking already dispatched to it**, or the
reviewer cannot reach the tour screens and will reject for incomplete
functionality.

## Before submitting

- [ ] EXT-3: Apple Developer Program membership active
- [ ] `npx eas init` has written `extra.eas.projectId` + `owner` to both `app.json`s
- [ ] `appleTeamId` and `ascAppId` filled in both `eas.json` files
- [ ] Privacy policy and terms URLs return 200 (currently placeholders — P9-06)
- [ ] Demo accounts created, with a dispatched booking for the guide account
- [ ] Support URL and marketing URL live
- [ ] Age rating completed
- [ ] `npx eas build --profile production --platform ios` succeeds
- [ ] Deletion verified on a real device against staging

Expect **1–7 days** of review. Submit before the launch window, not during it.
