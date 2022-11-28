create table if not exists balance
(
    user_id       varchar(36)    not null,
    currency      varchar(3)     not null,
    current_value numeric(10, 2) not null,
    constraint balance_pk
        primary key (user_id)
);
