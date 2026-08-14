-- Phase M (M-24): give legal_documents an actual body so the published pages
-- carry text rather than an outline, and make that text CMS-editable.
--
-- Editing NEVER mutates a row. consent_records references (document, version),
-- so a user who accepted privacy@2026-08-13 accepted *those words*; rewriting
-- them in place would silently re-point their consent at different text. The
-- admin editor therefore inserts a new version and leaves history intact.
--
-- `approved` is false until counsel signs off. The public site renders an
-- explicit draft banner while it is false, so the honesty is enforced by data
-- rather than by someone remembering to add a note.

ALTER TABLE legal_documents
    ADD COLUMN summary     text,
    ADD COLUMN body        text,
    ADD COLUMN approved    boolean NOT NULL DEFAULT false,
    ADD COLUMN approved_at timestamptz,
    ADD COLUMN approved_by uuid REFERENCES users (id);

-- Approval must record who and when, together.
ALTER TABLE legal_documents
    ADD CONSTRAINT legal_documents_approval_complete
    CHECK (
        (approved = false AND approved_at IS NULL AND approved_by IS NULL)
        OR (approved = true AND approved_at IS NOT NULL)
    );

CREATE INDEX idx_legal_documents_published ON legal_documents (document, published_at DESC);

-- ---------------------------------------------------------------------------
-- Initial content (version 2026-08-14).
--
-- Written to describe what the platform actually does — the retention rules,
-- the location window, the payment flow and the deletion behaviour below all
-- match the implementation. It is NOT legal advice and is not approved.
-- ---------------------------------------------------------------------------

INSERT INTO legal_documents (document, version, url, summary, body, approved) VALUES
(
    'terms',
    '2026-08-14',
    'https://proguidegh.com/legal/terms',
    'The agreement between you and ProGuideGH when you book or provide a guided tour.',
    $doc$
## 1. Who we are

ProGuideGH is a marketplace that connects travellers with certified tourist guides in Ghana. We operate the platform, verify and certify guides, set prices, take payment and provide operational support during tours. We are not ourselves a tour operator, and a guide is an independent professional rather than our employee.

Where these terms say "we" or "ProGuideGH" they mean the company operating this platform. "You" means whoever is using it, whether as a traveller or as a guide.

## 2. Using ProGuideGH

You must be at least 18 to hold an account. The information you give us must be accurate, and you are responsible for what happens under your account, so keep your password to yourself and tell us promptly if you think someone else has it.

One person, one account. Accounts are not transferable.

## 3. Booking a tour

Search results show guides who are currently certified and eligible. When you request a quote, **ProGuideGH calculates the price** — the guide does not set it and cannot change it. The quote you see is the amount you will be charged.

A booking is created in a pending state and is only confirmed once your payment provider tells us the payment succeeded. Until that confirmation arrives the booking is not secured, even if your bank has shown a pending charge.

You may not agree a side arrangement with a guide to take a tour off-platform that you found here. Doing so removes the protections in these terms — tracking, insurance evidence, receipts and dispute handling — from both of you.

## 4. Prices, fees and the tourism levy

Every quote and every receipt itemises:

- the tour price;
- the ProGuideGH platform commission; and
- the tourism levy applicable to tourism services in Ghana.

Rates are configuration, not code, and are effective-dated. If a rate changes, bookings already made keep the rate they were quoted at.

**No negotiation at the meeting point.** A guide who asks you for additional payment for the booked tour is in breach of their agreement with us; please report it. Tipping is welcome but never expected and never solicited.

## 5. Payment

Payment is taken on our payment provider's hosted page. **ProGuideGH never receives, sees or stores your card or Mobile Money credentials.** We receive confirmation that a payment succeeded, along with a provider reference.

You are responsible for any fees your own bank or mobile money operator charges you.

## 6. Cancellations and refunds

Cancellation windows and the refund due in each case are published in the app at the time of booking and shown before you confirm.

Approved refunds are issued against the original payment method as reversing entries in our ledger. We do not issue platform credit in place of a refund. Refunds initiated by us may take additional time to appear depending on your bank or mobile money operator.

If a guide cancels or does not appear, you are entitled to a full refund, and we will try to find you a replacement guide where the timing allows.

## 7. If you are a guide

To be certified you must complete identity verification and a background check, evidence your Ghana Tourism Authority registration, and complete the training modules for the specialties you want to work in.

You must keep your certification and any required documents current. **When a required document expires, you stop appearing in search on that day** — this is automatic.

You choose when to be available. Offers show the fee before you accept, and declining an offer costs you nothing and does not count against you. You may not accept a tour that overlaps one you already hold.

Earnings become eligible for payout after a tour completes and a short hold period passes. Payouts are made in a weekly batch to a payout account verified in your own name. You may not be paid to an account belonging to someone else.

ProGuideGH is not exclusive. You are free to take private work outside the platform.

## 8. Conduct

Everyone using ProGuideGH is expected to behave lawfully and with basic courtesy. We do not tolerate harassment, discrimination, intoxication on duty, or unsafe conduct during a tour.

Travellers should arrive at the meeting point at the agreed time and follow reasonable safety instructions from their guide, particularly at heritage sites, in markets and on the Kakum canopy walkway.

## 9. Safety and tracking

While a tour is active, the guide's location is shared with you and with our operations team so that you can find each other and so we can respond to problems. Your own location is not tracked. The location sharing policy explains this in detail.

Every active booking has an SOS button. **The SOS button alerts the ProGuideGH operations team. It is not an emergency service.** In an emergency, contact the Ghana Police Service on 191 or the National Ambulance Service on 193 first.

## 10. Reviews

Only a traveller who completed and paid for a tour may review it, and only once per booking. We may remove a review that is unlawful, defamatory, or unrelated to the tour, but we do not remove reviews for being unfavourable.

## 11. Suspension and termination

We may suspend or close an account that breaches these terms, fails certification, or presents a safety risk. Where we can, we will tell you why and give you a route to respond; where there is an immediate safety concern we may act first.

You may close your account at any time from within the app or from our website. Some records are retained after closure — the privacy policy explains which and why.

## 12. Liability

ProGuideGH provides the platform, certification process and operational support described here. We are responsible for our own failures in providing them.

A guide is an independent professional and is responsible for the conduct of the tour itself. Nothing in these terms excludes liability that cannot lawfully be excluded, including for death or personal injury caused by negligence, or for fraud.

## 13. Changes

We may change these terms. Material changes will be notified in the app before they take effect, and the version you accepted is recorded against your account. Continuing to use ProGuideGH after a change takes effect means you accept the new version.

## 14. Disputes and governing law

These terms are governed by the laws of the Republic of Ghana, and the courts of Ghana have jurisdiction.

Before going to court, please raise the matter with us — most disputes are resolved faster directly. Write to support@proguidegh.com.
$doc$,
    false
),
(
    'privacy',
    '2026-08-14',
    'https://proguidegh.com/legal/privacy',
    'What personal data ProGuideGH collects, why, who it is shared with, and the rights you have over it.',
    $doc$
## 1. Who controls your data

ProGuideGH is the data controller for the personal data described here. For any privacy question, or to exercise a right below, write to **privacy@proguidegh.com**.

## 2. What we collect, and why

**Account details** — your email address, phone number and a hashed password. Used to create and secure your account. We never store your password itself, only a hash of it.

**Profile details** — your name, nationality, preferred language, and an emergency contact if you give one. Used to introduce you to your guide and, in the case of the emergency contact, only if you raise an SOS during a tour.

**Booking records** — the tours you booked, when, where you met, how many guests, and the price. Used to run the booking and to meet our tax and tourism-levy obligations.

**Payment records** — the amount, currency, status and the payment provider's reference. **We never receive or store your card or Mobile Money credentials**; those are handled entirely by our payment provider on their own hosted page.

**Guide location** — for guides only, and only while online or on an active tour. Section 4 covers this in full.

**Guide verification documents** — identity documents, Ghana Tourism Authority registration and qualifications. Stored privately and reachable only through short-lived signed links. Used to certify guides and to keep certification current.

**Support and incident records** — what you told us, and what we did about it.

**Technical data** — the IP address a session was created from, and crash diagnostics. Used to secure accounts, detect session-token reuse and fix faults.

We do **not** use your data for advertising, we do **not** track you across other apps or websites, and we do **not** sell personal data.

## 3. Our lawful basis

We process your data because it is necessary to provide the service you asked for; because we have a legal obligation, particularly for financial records; because we have a legitimate interest in keeping the platform safe and preventing fraud; and, for guide location, on the basis of consent that you can withdraw.

## 4. Guide location data

This is the most sensitive thing the platform handles, so the rules are narrow and enforced in the software, not just stated here:

- **Only guides' locations are collected. Travellers are never tracked.**
- Collection happens only while a guide is online, or on a tour that has started and not yet finished.
- Position is sampled roughly every 15 seconds or 25 metres while active.
- On Android, a persistent notification is shown for the whole time background collection is running.
- The current position is held briefly in fast storage and expires automatically. Coarser checkpoints are retained for safety and audit for the period in our retention policy, then removed.
- It is visible to the traveller on that specific tour, and to our operations team. Nobody else. A guide's movement history is never shown to travellers.
- Collection stops when the tour completes or the guide goes offline.

## 5. Who we share data with

**Your counterparty** — a traveller and their assigned guide see each other's name and the details needed for the tour.

**Service providers** who process data on our behalf under contract: our payment provider, our cloud hosting and private file storage provider, and our email, SMS and push notification providers.

**Authorities**, where we are legally required to disclose, or where it is necessary to protect someone's safety.

We do not share your data with anyone else.

## 6. Where your data is held

Our infrastructure providers may store or process data outside Ghana. Where that happens we rely on the contractual protections in our agreements with them.

## 7. How long we keep it

Account and profile data is kept while your account is open.

When you delete your account we **irreversibly remove** your name, email, phone number, password, emergency contact, verification documents (including the stored files), payout account details and location history, and we revoke every session immediately.

We **retain** payment records, receipts and ledger entries, because Ghanaian tax and tourism-levy obligations require it and that obligation does not end when an account closes. After deletion these reference an account identifier that no longer identifies you. Reviews you wrote remain visible, because other travellers rely on them, but they are no longer linked to your name.

## 8. Your rights

You can:

- **See what we hold** — export all your data from within the app at any time.
- **Correct it** — edit your profile directly, or write to us.
- **Delete it** — delete your account from the app or from our website. Deletion is refused only temporarily, and with the specific reason shown, if you have an unfinished booking or unpaid earnings.
- **Withdraw consent** — turn off location sharing at any time. Guides who do will not receive dispatched work while it is off.
- **Complain** — to us first, and to Ghana's Data Protection Commission if we have not resolved it.

We do not charge for any of this and we will not make you justify the request.

## 9. How we protect it

Passwords are hashed with a modern algorithm. Sessions are short-lived and rotate, and reuse of a rotated token revokes the whole session family. Administrators with access to financial or personal data must use multi-factor authentication, and every privileged action is written to an append-only audit log. Verification documents are never public and are only reachable through links that expire.

## 10. Children

ProGuideGH is not for anyone under 18 and we do not knowingly collect data from children. If you believe a child has an account, tell us and we will remove it.

## 11. Changes

If we change this policy materially we will tell you in the app before the change takes effect, and record which version you accepted.

## 12. Contact

**privacy@proguidegh.com** — privacy questions, data requests, and anything on this page.
$doc$,
    false
),
(
    'location',
    '2026-08-14',
    'https://proguidegh.com/legal/location',
    'The specific rules governing guide location data — the most sensitive thing the platform handles.',
    $doc$
## Why this is a separate document

Location is the one thing ProGuideGH collects that could genuinely harm someone if it were mishandled. It deserves its own page rather than a clause buried in the privacy policy.

## Whose location

**Guides only.** A traveller's location is never collected, never stored and never shared. If you are booking a tour, this document does not describe anything happening to you.

## When it is collected

Only in two situations:

1. While a guide has chosen to **go online** and is available for work.
2. While a guide is on a tour that has **started and not yet finished** — from setting off to meet the traveller through to marking the tour complete.

Outside those windows nothing is collected. Not on a cold start, not between tours, not while offline. This is enforced in the app: when there is no active tour the software discards location readings rather than sending them.

## What is collected

Latitude and longitude, an accuracy estimate, and where the device provides them, heading and speed, with the time of the reading. Sampled roughly every 15 seconds or every 25 metres of movement.

## Background collection

Guides need location to keep working when the screen is off — a phone locks in a pocket during a tour, and a traveller losing sight of their guide at that moment is the problem this solves.

On **Android** a persistent notification is displayed for the entire time background collection is running. It is not dismissible while active, by design: you should always be able to tell at a glance whether you are sharing.

On **iOS** the system shows its own indicator when an app uses location in the background, and iOS will periodically remind you that ProGuideGH has this permission.

## Who can see it

- **The traveller on that specific tour**, while it is active, so they can find you.
- **The ProGuideGH operations team**, for dispatch and emergency response.

Nobody else. A guide's position is never shown publicly, never shown on a guide's public profile, and a guide's historical movement is never shown to travellers.

## How long it is kept

The live position is held in fast, temporary storage and expires automatically within about a minute if no newer reading arrives.

Coarser checkpoints are kept for the retention period in our privacy policy so that a completed tour can be reconstructed if there is a safety incident or a dispute. After that they are removed.

**When a guide deletes their account, their entire location history is deleted with it.**

## Turning it off

Location sharing can be turned off at any time, from the app or from your phone's settings.

The consequence is straightforward and we would rather state it than surprise you: **dispatch cannot send you nearby jobs if it does not know where you are.** You can still be booked directly and can still run tours, but you will not receive dispatched offers while sharing is off.

## What we never do

We do not sell location data. We do not use it for advertising. We do not share it with data brokers. We do not use it to rank or penalise guides. And we do not collect it from travellers at all.

## Questions

**privacy@proguidegh.com**
$doc$,
    false
)
ON CONFLICT (document, version) DO NOTHING;
