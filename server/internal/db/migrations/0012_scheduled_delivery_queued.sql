-- 0012_scheduled_delivery_queued.sql — IC-012.
--
-- 0002 constrained scheduled_deliveries.status to ('sent','failed','skipped').
-- Those three were the whole story while the sweep talked to APNs and FCM
-- itself: by the time a row was written the send had already happened, and
-- "sent" meant the provider had accepted it.
--
-- Production now queues each delivery (see scheduler.Queue), so the row is
-- written before any provider has been contacted. Recording that as "sent"
-- would be a lie with consequences: the one question this table is opened to
-- answer is whether a given reminder reached a phone, and a row claiming
-- "sent" for a message still sitting in the job queue answers it wrongly —
-- during exactly the incident when somebody is asking.
--
-- "queued" is therefore its own state: the occurrence has been handled and the
-- sweep will not attempt it again, while the queue owns delivery from here. The
-- final outcome lands in jobs.status, where retries and dead letters are
-- already tracked.
--
-- This migration only widens the vocabulary. Unlike 0003, 0009 and 0010 there
-- is nothing to normalise first, because every value already stored remains
-- valid under the new constraint.

ALTER TABLE scheduled_deliveries DROP CONSTRAINT IF EXISTS scheduled_deliveries_status_check;
ALTER TABLE scheduled_deliveries ADD CONSTRAINT scheduled_deliveries_status_check
    CHECK (status IN ('sent','queued','failed','skipped'));
