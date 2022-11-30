create table transaction
(
    id                       int8                                not null,
    transaction_currency     varchar(3)                          not null,
    transaction_value        numeric(10, 2)                      not null,
    sender_id                varchar(36),
    sender_currency          varchar(3),
    sender_value             numeric(10, 2),
    sender_balance_before    numeric(10, 2),
    sender_balance_after     numeric(10, 2),
    recipient_id             varchar(36),
    recipient_currency       varchar(3),
    recipient_value          numeric(10, 2),
    recipient_balance_before numeric(10, 2),
    recipient_balance_after  numeric(10, 2),
    merchant_data            jsonb,
    order_data               jsonb,
    created_at               timestamp default CURRENT_TIMESTAMP not null,
    constraint transaction_pk
        primary key (id),
    constraint transaction_balance__fk
        foreign key (recipient_id) references balance (user_id)
            on update restrict on delete restrict,
    constraint transaction_balance_sender_id_fk
        foreign key (sender_id) references balance (user_id)
            on update restrict on delete restrict,
    constraint transaction_sender_id_recipient_id_check
        check (sender_id is not null or recipient_id is not null)
);

create index transaction_order_data_item_id_index
    on transaction ((date_trunc('month', created_at)), (order_data ->> 'item_id'))
    where (order_data ->> 'item_id') is not null;

create index transaction_sender_id_created_at_index
    on transaction (sender_id, created_at);

create index transaction_recipient_id_created_at_index
    on transaction (recipient_id, created_at);
