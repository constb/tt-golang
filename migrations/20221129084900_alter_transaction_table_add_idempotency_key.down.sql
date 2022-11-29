drop index transaction_idempotency_key_index;

alter table transaction
    drop column idempotency_key;
