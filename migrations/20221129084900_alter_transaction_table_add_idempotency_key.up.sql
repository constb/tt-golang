alter table transaction
    add column idempotency_key varchar(36);

create unique index transaction_idempotency_key_index
    on transaction (idempotency_key)
    where idempotency_key is not null;
