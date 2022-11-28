create table balance_reserve
(
    order_id            varchar(36)                         not null,
    user_id             varchar(36)                         not null,
    item_id             varchar(36)                         not null,
    currency            varchar(3)                          not null,
    value               numeric(10, 2)                      not null,
    user_currency_value numeric(10, 2)                      not null,
    created_at          timestamp default CURRENT_TIMESTAMP not null,
    constraint balance_reserve_pk
        primary key (order_id),
    constraint balance_reserve_balance_user_id_fk
        foreign key (user_id) references balance (user_id)
);

create index balance_reserve_created_at_index
    on balance_reserve (created_at);
