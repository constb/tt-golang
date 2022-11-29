create unique index transaction_order_id_index
    on "transaction" ((order_data ->> 'order_id'))
    where (order_data ->> 'order_id') is not null;
